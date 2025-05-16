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

package pigeon

import (
	"fmt"
	"net"
	"os"

	"gopkg.in/yaml.v3"
)

const secretToken = "<secret>"

type Secret string

func (s Secret) MarshalYAML() (interface{}, error) {
	if s == "" {
		return "", nil
	}
	return secretToken, nil
}

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

type HostPort struct {
	Host string
	Port string
}

func (hp *HostPort) UmarshalYAML(unmarshal func(interface{}) error) error {
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

func (hp HostPort) MarshalYAML() (interface{}, error) {
	return hp.String(), nil
}

func (hp HostPort) String() string {
	if hp.Host == "" && hp.Port == "" {
		return ""
	}
	return fmt.Sprintf("%s:%s", hp.Host, hp.Port)
}

type EmailConfig struct {
	From         string            `yaml:"from,omitempty" json:"from,omitempty"`
	To           string            `yaml:"to,omitempty" json:"to,omitempty"`
	Cc           string            `yaml:"cc,omitempty" json:"cc,omitempty"`
	Bcc          string            `yaml:"bcc,omitempty" json:"bcc,omitempty"`
	Hello        string            `yaml:"hello,omitempty" json:"hello,omitempty"`
	Smarthost    HostPort          `yaml:"smarthost,omitempty" json:"smarthost,omitempty"` // host:port
	AuthUsername string            `yaml:"auth_username,omitempty" json:"auth_username,omitempty"`
	AuthPassword Secret            `yaml:"auth_password,omitempty" json:"auth_password,omitempty"`
	Headers      map[string]string `yaml:"headers,omitempty" json:"headers,omitempty"`
	RequireTLS   *bool             `yaml:"require_tls,omitempty" json:"require_tls,omitempty"`
	Text         string            `yaml:"text,omitempty" json:"text,omitempty"`
	HTML         string            `yaml:"html,omitempty" json:"html,omitempty"`
	TemplatePath string            `yaml:"template_path,omitempty" json:"template_path,omitempty"`
}

func Load(s string) (*EmailConfig, error) {
	var cfg EmailConfig
	if err := yaml.Unmarshal([]byte(s), &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func LoadFile(filename string) (*EmailConfig, error) {
	b, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	return Load(string(b))
}

func (c *EmailConfig) String() string {
	b, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Sprintf("<error creating config string: %s", err)
	}

	return string(b)
}
