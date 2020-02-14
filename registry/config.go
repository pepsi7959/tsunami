package tsregistry

//Conf is global configurations
type Conf struct {
	ID              string
	Name            string
	Endpoint        string
	MaxConnections  int
	MaxConcurrences int
}
