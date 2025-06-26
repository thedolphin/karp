-- supplemental functions
function PrintTable(t)
    for key, value in pairs(t) do
        if type(value) == "table" then
            print(key .. " = {")
            PrintTable(value)
            print("}")
        else
            print(key .. " = " .. tostring(value))
        end
    end
end

--- Process each message
-- @param message table with folowing fields:
--         * Topic - topic name
--         * Value - message value
--         * User - username used for connection
--         * Group - consumer group used for connection
-- @return value may be:
--         * true - pass message as is to client
--         * false - skip message
--         * string - modified value to be sent to the client

function Process(message)

    PrintTable(message)

    -- just check json field value
    if message.Topic == "my.test.topic" then
        local value = yyjson.load(message.Value)
        if value.status == "INPROGRESS" then
            return false
        end
    end

    -- mangle json document for custom group
    if message.Topic == "my.test.topic" and message.Group == "analytic" then
        local value = yyjson.load_mut(message.Value)
        value["secret_field"] = nil
        value.meta.test = "test"
        return tostring(value)
    end

    -- any other messages will be passed as is
    return true
end