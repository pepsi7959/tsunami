package tshttp

//Conf configuration structure
type Conf struct {
	Name        string
	URL         string
	Protocol    string
	Host        string
	Port        string
	Path        string
	Method      string
	Headers     map[string]string
	Body        string
	Concurrence int
	MaxConns    int
	MaxQueues   int
}
