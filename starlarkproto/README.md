## file

`file(name)` returns the descriptor of the proto file.

| Parameter | Description |
| ------------- | ------------- |
| name | string <br /> Name of the proto file. |

## new_message

`new_message(name)` returns a new zero initialised protobuffer of the typed named.

## marshal

`marshal(msg)` message as a binary encoded string.

## unmarshal

`unmarshal(b, msg)` protobuffer into the msg.

## marshal_json

`marshal_json(msg)` message as JSON.

## unmarshal_json

`unmarshal_json(b, msg)` json into the msg.

## marshal_text

`marshal_text(msg)` message as text.

## unmarshal_text

`unmarshal_text(b, msg)` text into the msg.
