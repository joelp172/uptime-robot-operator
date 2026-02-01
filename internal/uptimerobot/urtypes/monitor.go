package urtypes

const (
	MonitorPaused = iota
	MonitorRunning
)

// API string constants for monitor types.
const (
	APITypeHTTP      = "HTTP"
	APITypeKeyword   = "Keyword"
	APITypePing      = "Ping"
	APITypePort      = "Port"
	APITypeHeartbeat = "Heartbeat"
	APITypeDNS       = "DNS"
)

//go:generate go run github.com/dmarkham/enumer -type MonitorType -trimprefix Type -json -text

//+kubebuilder:validation:Type:=string
//+kubebuilder:validation:Enum:=HTTPS;Keyword;Ping;Port;Heartbeat;DNS

type MonitorType uint8

const (
	TypeHTTPS MonitorType = iota + 1
	TypeKeyword
	TypePing
	TypePort
	TypeHeartbeat
	TypeDNS
)

// ToAPIString returns the v3 API string representation of the monitor type.
func (m MonitorType) ToAPIString() string {
	switch m {
	case TypeHTTPS:
		return APITypeHTTP
	case TypeKeyword:
		return APITypeKeyword
	case TypePing:
		return APITypePing
	case TypePort:
		return APITypePort
	case TypeHeartbeat:
		return APITypeHeartbeat
	case TypeDNS:
		return APITypeDNS
	default:
		return APITypeHTTP
	}
}

// MonitorTypeFromAPIString converts a v3 API string to MonitorType.
func MonitorTypeFromAPIString(s string) MonitorType {
	switch s {
	case APITypeHTTP:
		return TypeHTTPS
	case APITypeKeyword:
		return TypeKeyword
	case APITypePing:
		return TypePing
	case APITypePort:
		return TypePort
	case APITypeHeartbeat:
		return TypeHeartbeat
	case APITypeDNS:
		return TypeDNS
	default:
		return TypeHTTPS
	}
}

//go:generate go run github.com/dmarkham/enumer -type MonitorAuthType -trimprefix Auth -json -text

//+kubebuilder:validation:Type:=string
//+kubebuilder:validation:Enum:=Basic;Digest

type MonitorAuthType uint8

const (
	AuthBasic MonitorAuthType = iota + 1
	AuthDigest
)
