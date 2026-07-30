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
	Verbose     bool

	// InsecureSkipVerify ข้ามการตรวจสอบ TLS certificate ของ target
	// เปิดไว้เป็น default เพราะ target ที่ทดสอบมักเป็น endpoint ภายในที่ใช้
	// self-signed cert, cert หมดอายุ, หรือถูกยิงด้วย IP ตรง ๆ ซึ่ง hostname ไม่ match
	InsecureSkipVerify bool
}
