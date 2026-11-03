package constants

type StrmFileType uint8

const (
	UnknownStrm StrmFileType = iota
	HTTPStrm
)

func (s StrmFileType) String() string {
	switch s {
	case HTTPStrm:
		return "HTTPStrm"
	default:
		return "UnknownStrm"
	}
}
