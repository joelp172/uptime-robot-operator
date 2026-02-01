package urtypes

const (
	MonitorPaused = iota
	MonitorRunning
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
		return "HTTP"
	case TypeKeyword:
		return "Keyword"
	case TypePing:
		return "Ping"
	case TypePort:
		return "Port"
	case TypeHeartbeat:
		return "Heartbeat"
	case TypeDNS:
		return "DNS"
	default:
		return "HTTP"
	}
}

// MonitorTypeFromAPIString converts a v3 API string to MonitorType.
func MonitorTypeFromAPIString(s string) MonitorType {
	switch s {
	case "HTTP":
		return TypeHTTPS
	case "Keyword":
		return TypeKeyword
	case "Ping":
		return TypePing
	case "Port":
		return TypePort
	case "Heartbeat":
		return TypeHeartbeat
	case "DNS":
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
