package runtime

type custom struct {
	Config
}

func (p custom) SanitizeDependencies(dependencies string) string {
	return dependencies
}

var _ Runtime = custom{}
