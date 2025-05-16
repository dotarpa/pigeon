// Copyright 2015 Prometheus Team
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package pigeon provides an email sending library with support for templates,
// YAML configuration, and attachments.
package pigeon

import (
	"fmt"
	"net"
	"os"

	"gopkg.in/yaml.v3"
)

const secretToken = "<secret>"

// Secret is a string that is hidden when marshaled back to YAML/JSON.
// It is typically used for sensitive information such as passwords or API tokens.
type Secret string

// MarshalYAML implements yaml.Marshaler.
// It always outputs "<secret>" when marshaled to YAML.
func (s Secret) MarshalYAML() (interface{}, error) {
	if s == "" {
		return "", nil
	}
	return secretToken, nil
}

// UnmarshalYAML implements yaml.Unmarshaler.
// It sets the value unless the raw string equals "<secret>".
func (s *Secret) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var raw string
	if err := unmarshal(&raw); err != nil {
		return err
	}

	if raw == secretToken {
		return nil
	}

	*s = Secret(raw)
	return nil
}

// HostPort represents an SMTP smarthost as "host:port".
// Used for the Smarthost field in EmailConfig.
type HostPort struct {
	Host string
	Port string
}

// UnmarshalYAML implements yaml.Unmarshaler for HostPort.
// It splits a "host:port" string and sets the Host and Port fields.
func (hp *HostPort) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var (
		raw string
		err error
	)
	if err := unmarshal(&raw); err != nil {
		return err
	}

	if raw == "" {
		hp.Host, hp.Port = "", ""
		return nil
	}

	hp.Host, hp.Port, err = net.SplitHostPort(raw)
	if err != nil {
		return err
	}
	if hp.Port == "" {
		return fmt.Errorf("address %q: port cannot be empty", raw)
	}

	return nil
}

// MarshalYAML implements yaml.Marshaler for HostPort.
func (hp HostPort) MarshalYAML() (interface{}, error) {
	return hp.String(), nil
}

// String returns the "host:port" representation of the HostPort.
func (hp HostPort) String() string {
	if hp.Host == "" && hp.Port == "" {
		return ""
	}
	return fmt.Sprintf("%s:%s", hp.Host, hp.Port)
}

// EmailConfig holds all configuration for sending an email.
// It can be loaded from a YAML file using Load or LoadFile.
type EmailConfig struct {
	// From specifies the sender's email address.
	From string `yaml:"from,omitempty" json:"from,omitempty"`
	// To specifies the primary recipients' addresses (comma-separated).
	To string `yaml:"to,omitempty" json:"to,omitempty"`
	// Cc specifies the CC recipients' addresses (comma-separated).
	Cc string `yaml:"cc,omitempty" json:"cc,omitempty"`
	// Bcc specifies the BCC recipients' addresses (comma-separated).
	Bcc string `yaml:"bcc,omitempty" json:"bcc,omitempty"`
	// Hello specifies the value for the SMTP HELO/EHLO command.
	Hello string `yaml:"hello,omitempty" json:"hello,omitempty"`
	// Smarthost specifies the SMTP relay host as "host:port".
	Smarthost HostPort `yaml:"smarthost,omitempty" json:"smarthost,omitempty"` // host:port
	// AuthUsername specifies the username for SMTP authentication (if needed).
	AuthUsername string `yaml:"auth_username,omitempty" json:"auth_username,omitempty"`
	// AuthPassword specifies the password for SMTP authentication (if needed).
	AuthPassword Secret `yaml:"auth_password,omitempty" json:"auth_password,omitempty"`
	// Headers allows custom headers to be set in the message.
	Headers map[string]string `yaml:"headers,omitempty" json:"headers,omitempty"`
	// RequireTLS forces the use of TLS when connecting to the SMTP server (optional).
	RequireTLS *bool `yaml:"require_tls,omitempty" json:"require_tls,omitempty"`
	// Text can be used to directly set the plain text body (optional).
	Text string `yaml:"text,omitempty" json:"text,omitempty"`
	// HTML can be used to directly set the HTML body (optional, for future use).
	HTML string `yaml:"html,omitempty" json:"html,omitempty"`
	// Timezone specifies the IANA time zone to use for the Date header (e.g., "Asia/Tokyo").
	Timezone string `yaml:"timezone,omitempty" json:"timezone,omitempty"`

	// Attachments is a list of file paths to be attached to the email.
	Attachments []string `yaml:"attachments,omitempty" json:"attachments,omitempty"`
	// TemplatePath specifies the file path to the email template.
	TemplatePath string `yaml:"template_path,omitempty" json:"template_path,omitempty"`
}

// Load parses the YAML string s and returns a new EmailConfig instance.
// Returns an error if the input is not valid YAML or configuration.
func Load(s string) (*EmailConfig, error) {
	var cfg EmailConfig
	if err := yaml.Unmarshal([]byte(s), &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// LoadFile reads and parses the YAML file at the given filename,
// returning an EmailConfig. Returns an error if reading or parsing fails.
func LoadFile(filename string) (*EmailConfig, error) {
	b, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	return Load(string(b))
}

// String returns a redacted YAML representation of the configuration,
// hiding secret fields.
func (c *EmailConfig) String() string {
	b, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Sprintf("<error creating config string: %s", err)
	}

	return string(b)
}
