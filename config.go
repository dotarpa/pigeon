package pigeon

type EmailConfig struct {
	From         string            `yaml:"from,omitempty" json:"from,omitempty"`
	To           string            `yaml:"to,omitempty" json:"to,omitempty"`
	Cc           string            `yaml:"cc,omitempty" json:"cc,omitempty"`
	Bcc          string            `yaml:"bcc,omitempty" json:"bcc,omitempty"`
	Hello        string            `yaml:"hello,omitempty" json:"hello,omitempty"`
	Smarthost    string            `yaml:"smarthost,omitempty" json:"smarthost,omitempty"` // host:port
	AuthUsername string            `yaml:"auth_username,omitempty" json:"auth_username,omitempty"`
	AuthPassword string            `yaml:"auth_password,omitempty" json:"auth_password,omitempty"`
	Headers      map[string]string `yaml:"headers,omitempty" json:"headers,omitempty"`
	RequireTLS   *bool             `yaml:"require_tls,omitempty" json:"require_tls,omitempty"`
	Text         string            `yaml:"text,omitempty" json:"text,omitempty"`
	HTML         string            `yaml:"html,omitempty" json:"html,omitempty"`
	TemplatePath string            `yaml:"template_path,omitempty" json:"template_path,omitempty"`
}
