package dockerlogbeat

func CopySlice(src []byte) []byte {
	if src == nil {
		return nil
	}
	result := make([]byte, len(src))
	copy(result, src)
	return result
}

func CopyDleSlice(src []*DockerLogEvent) []*DockerLogEvent {
	result := make([]*DockerLogEvent, len(src))
	copy(result, src)
	return result
}

func SliceContains(self []string, str string) bool {
	for _, item := range self {
		if item == str {
			return true
		}
	}
	return false
}
