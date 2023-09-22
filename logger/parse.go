package logger

const (
	WatchOffLevelString     = "off"
	WatchNoneLevelString    = "none"
	WatchDefaultLevelString = "default"
)

func ParseWatchLogLevel(level string) WatchLogLevel {
	switch level {
	case "micro":
		return MicroLevel
	case "trace", "T":
		return TraceLevel
	case "debug", "D":
		return DebugLevel
	case "info", "I":
		return InfoLevel
	case "note", "N":
		return NoteLevel
	case "warn", "warning", "W":
		return WarnLevel
	case "crit", "critical", "error", "err", "C", "E":
		return ErrorLevel
	case "off", "none":
		return OffLevel
	case "default", "def":
		fallthrough
	default:
		return DefaultLevel
	}
}

func GetWatchLogLevelString(level WatchLogLevel) string {
	switch level {
	case MicroLevel:
		return "micro"
	case TraceLevel:
		return "trace"
	case DebugLevel:
		return "debug"
	case InfoLevel:
		return "info"
	case NoteLevel:
		return "note"
	case WarnLevel:
		return "warn"
	case ErrorLevel:
		return "crit"
	case OffLevel:
		return "off"
	default:
		Panicf("Unknown WatchLogLevel: %d", level)
		return ""
	}
}
