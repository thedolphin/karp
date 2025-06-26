package karpserver

import (
	"errors"
	"fmt"

	"github.com/IBM/sarama"
	"github.com/thedolphin/luarunner"
)

func luaInit(code string) (*luarunner.LuaRunner, error) {

	lua, err := luarunner.New()
	if err != nil {
		return nil, err
	}

	lua.StrictRead()

	err = lua.Load(code)
	if err != nil {
		lua.Close()
		return nil, err
	}

	err = lua.Run()
	if err != nil {
		lua.Close()
		return nil, err
	}

	if !lua.CheckFunction("Process") {
		lua.Close()
		return nil, errors.New("'Process' function not found")
	}

	lua.StrictWrite()

	return lua, nil
}

func luaProcess(
	lua *luarunner.LuaRunner,
	msg *sarama.ConsumerMessage,
	user, group string,
) (
	bool, error,
) {

	lua.GetGlobal("Process")
	lua.Push(map[string]any{
		"Topic": msg.Topic,
		"Value": msg.Value,
		"User":  user,
		"Group": group,
	})

	err := lua.Call(1, 1)
	if err != nil {
		return false, fmt.Errorf("error calling Process: %w", err)
	}

	vAny, err := lua.Pop()
	if err != nil {
		return false, fmt.Errorf("error getting return value: %w", err)
	}

	switch v := vAny.(type) {
	case string:
		msg.Value = []byte(v)
		return true, nil
	case bool:
		return v, nil
	default:
		return false, fmt.Errorf("return value (expecting new value as string or bool): %v", vAny)
	}
}
