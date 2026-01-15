package karp

import (
	context "context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	reflect "reflect"
	sync "sync"
	"time"

	grpc "google.golang.org/grpc"
	codes "google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/metadata"
	status "google.golang.org/grpc/status"
)

// Client connects to Karp Server and consumes messages
type Client struct {
	messages   chan *Message
	conn       *grpc.ClientConn
	stream     grpc.BidiStreamingClient[Ack, Message]
	commitlock sync.Mutex
}

// ClientConfig is used to configure Client
// Can be loaded using https://github.com/ilyakaznacheev/cleanenv
type ClientConfig struct {
	ClientID        string   `yaml:"clientid" env-required:"true"` // identify client on Karp Server and Kafka Server (as a part of Member ID)
	Endpoint        string   `yaml:"endpoint" env-required:"true"` // string containing address and port of Karp Server
	UseTLS          bool     `yaml:"tls"`                          // whether TLS is used when connecting to the Karp Server
	TrustServerCert bool     `yaml:"trust"`                        // skip server certificate verification if set to true
	ClientCertPEM   string   `yaml:"clientcert"`                   // string containing client certificate in PEM format
	ClientKeyPEM    string   `yaml:"clientkey"`                    // string containing client certificate key in PEM format
	RootCAsPEM      string   `yaml:"rootca"`                       // string containing CA certificate in PEM format
	Cluster         string   `yaml:"cluster" env-required:"true"`  // cluster name, configured on server, to connect to
	Group           string   `yaml:"group" env-required:"true"`    // client Consumer Group name
	User            string   `yaml:"user"`                         // Kafka user name
	Password        string   `yaml:"password"`                     // Kafka user password
	Topics          []string `yaml:"topics" env-required:"true"`   // list of topics to subscribe
	Compression     string   `yaml:"compression"`                  // Compression type: snappy, lz4, gzip, zstd
}

// Messages returns channel of *Message
// Channel is closed when error occurs, so there's no need
// to break consuming cycle on error
func (kc *Client) Messages() <-chan *Message {
	return kc.messages
}

// Commit sends offset of Message to Karp Server to commit to Consumer Group offsets
func (kc *Client) Commit(msg *Message) error {

	// Just to be safe, we add a lock because Commit
	// is used in Consume, and calling Send from multiple
	// concurrent goroutines is not allowed.
	kc.commitlock.Lock()
	defer kc.commitlock.Unlock()

	return kc.stream.Send(&Ack{
		ID:        msg.ID,
		Topic:     msg.Topic,
		Partition: msg.Partition,
		Offset:    msg.Offset,
	})
}

// Close gracefully closes the connection.
// Make sure to stop Consume by cancelling its context
// before calling Close. Close will wait messages channel
// to close
func (kc *Client) Close() error {

	defer kc.conn.Close()

	var err error

	// send END_STREAM, server should close connection
	if err = kc.stream.CloseSend(); err != nil {
		return fmt.Errorf("error sending END_STREAM while closing gRPC connection: %w", err)
	}

	// unblock Consume and ensure it finished
	for range kc.messages {
	}

	// catch the rest of messages
	for err == nil {
		_, err = kc.stream.Recv()
	}

	if errors.Is(err, io.EOF) {
		return nil
	}

	return fmt.Errorf("error receiving message while closing gRPC connection: %w", err)
}

// Consume starts a blocking consumption loop.
// When an error occurs, it closes the messages channel and returns the reason.
// You should run it in a separate goroutine.
// Cancelling the provided context only stops the consumption loop;
// you must call Close afterwards to shut down the connection gracefully.
func (kc *Client) Consume(ctx context.Context) error {

	defer close(kc.messages)

	for {

		msg, err := kc.stream.Recv()

		if errors.Is(err, io.EOF) ||
			status.Code(err) == codes.Canceled ||
			(msg != nil && msg.Done) {

			return nil
		}

		if err != nil {
			return fmt.Errorf("error receiving message from server: %w", err)
		}

		// If this is an echo, send a reply.
		if len(msg.Topic) == 0 {
			kc.Commit(msg)
		} else {
			kc.messages <- msg
		}

		if err := ctx.Err(); err != nil {
			return nil
		}
	}
}

// ValidateClientConfig validates the ClientConfig according to struct tags.
// You must use it with NewClientFromConn() if the ClientConfig struct
// was filled manually.
func ValidateClientConfig(cfg *ClientConfig) error {

	v := reflect.ValueOf(cfg).Elem()
	t := v.Type()

	for i := range v.NumField() {
		field := t.Field(i)

		if field.Tag.Get("env-required") == "true" {
			if v.Field(i).Len() == 0 {
				return fmt.Errorf("required field %s is empty", field.Name)
			}
		}

		if field.Tag.Get("env-default") == "true" {
			v.Field(i).SetBool(true)
		}
	}

	return nil
}

func buildDialOptions(cfg *ClientConfig) ([]grpc.DialOption, error) {

	var dialOpts []grpc.DialOption

	if !cfg.UseTLS {
		dialOpts = append(dialOpts, grpc.WithTransportCredentials(insecure.NewCredentials()))

	} else {
		tlsCfg := &tls.Config{
			InsecureSkipVerify: cfg.TrustServerCert,
			NextProtos:         []string{"h2"},
		}

		if len(cfg.ClientCertPEM) > 0 && len(cfg.ClientKeyPEM) > 0 {
			cert, err := tls.X509KeyPair([]byte(cfg.ClientCertPEM), []byte(cfg.ClientKeyPEM))
			if err != nil {
				return nil, fmt.Errorf("failed to parse client cert/key: %w", err)
			}
			tlsCfg.Certificates = []tls.Certificate{cert}
		}

		if len(cfg.RootCAsPEM) > 0 {
			rootPool := x509.NewCertPool()
			if ok := rootPool.AppendCertsFromPEM([]byte(cfg.RootCAsPEM)); !ok {
				return nil, fmt.Errorf("failed to parse root CA")
			}
			tlsCfg.RootCAs = rootPool
		}

		dialOpts = append(dialOpts, grpc.WithTransportCredentials(credentials.NewTLS(tlsCfg)))
	}

	dialOpts = append(dialOpts, grpc.WithKeepaliveParams(keepalive.ClientParameters{
		Time:                15 * time.Second,
		Timeout:             5 * time.Second,
		PermitWithoutStream: true,
	}))

	return dialOpts, nil
}

// NewClientFromConn allows you to specify your own confugured grpc.ClientConn
// You must use ValidateClientConfig before calling it if
// ClientConfig struct filled manually
func NewClientFromConn(cfg *ClientConfig, conn *grpc.ClientConn) (*Client, error) {

	md := metadata.Pairs(
		"clientid", cfg.ClientID,
		"cluster", cfg.Cluster,
		"group", cfg.Group,
		"user", cfg.User,
		"password", cfg.Password,
	)

	md.Append("topic", cfg.Topics...)

	kc := &Client{}

	connCtx := metadata.NewOutgoingContext(context.Background(), md)
	client := NewKarpClient(conn)

	var err error
	var callOpts []grpc.CallOption
	if cfg.Compression != "" {
		callOpts = append(callOpts, grpc.UseCompressor(cfg.Compression))
	}

	kc.stream, err = client.Consume(connCtx, callOpts...)
	if err != nil {
		conn.Close()
		return nil, err
	}

	kc.conn = conn
	kc.messages = make(chan *Message, 128)

	return kc, nil
}

// NewClient returns a Client ready to consume
func NewClient(cfg *ClientConfig) (*Client, error) {

	if err := ValidateClientConfig(cfg); err != nil {
		return nil, err
	}

	dialOpts, err := buildDialOptions(cfg)
	if err != nil {
		return nil, err
	}

	conn, err := grpc.NewClient(cfg.Endpoint, dialOpts...)
	if err != nil {
		conn.Close()
		return nil, err
	}

	return NewClientFromConn(cfg, conn)
}
