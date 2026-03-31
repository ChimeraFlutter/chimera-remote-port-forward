package protocol

// ClientMessage 客户端发送给服务端的消息
type ClientMessage struct {
	Type       string `json:"type"`        // register, heartbeat, data
	DeviceName string `json:"device_name"` // 设备名称
	LocalPort  int    `json:"local_port"`  // 本地端口
	Token      string `json:"token"`       // 认证Token (仅register时使用)
	ConnID     string `json:"conn_id"`     // 连接标识 (data时使用)
	Data       []byte `json:"data"`        // 转发数据
}

// ServerMessage 服务端发送给客户端的消息
type ServerMessage struct {
	Type       string `json:"type"`        // assigned, heartbeat_ack, data, error
	RemotePort int    `json:"remote_port"` // 分配的远程端口
	ConnID     string `json:"conn_id"`     // 连接标识 (data时使用)
	Data       []byte `json:"data"`        // 转发数据
	Message    string `json:"message"`     // 错误信息
}

// 消息类型常量
const (
	// 客户端消息类型
	TypeRegister  = "register"
	TypeHeartbeat = "heartbeat"
	TypeData      = "data"

	// 服务端消息类型
	TypeAssigned     = "assigned"
	TypeHeartbeatAck = "heartbeat_ack"
	TypeConnOpen     = "conn_open"
	TypeConnClose    = "conn_close"
	TypeError        = "error"
)
