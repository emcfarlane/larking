Module defines these types:
- descriptor
- message
- enum
- list
- map

## file

`file(name)` returns the descriptor of the proto file.

| Parameter | Type | Description |
| --------- | ---- | ----------- |
| name | string | Name of the proto file. |

## new

`new(name)` returns a protobuf descriptor for the given type.

| Parameter | Type | Description |
| --------- | ---- | ----------- |
| name | string | Descriptor fullname of proto type. |

## marshal

`marshal(msg)` message as a binary encoded string.

| Parameter | Type | Description |
| --------- | ---- | ----------- |
| msg | message | Descriptor fullname of proto type. |

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

### message·field

`m.<field>` returns the value of the field named by the proto name.
