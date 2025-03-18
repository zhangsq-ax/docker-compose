package config

import (
	"fmt"
	"github.com/docker/cli/cli/compose/types"
	jsoniter "github.com/json-iterator/go"
	"gopkg.in/yaml.v3"
	"os"
	"path/filepath"
	"regexp"
	"sort"
)

var (
	regServiceImage = regexp.MustCompile(`^(.+?)(:([^/]+?))?$`)
	regEnv          = regexp.MustCompile(`^(.+?)=(.+?)$`)
)

type ComposeNetworkConfig struct {
	Name     string `yaml:"name,omitempty" json:"name,omitempty"`
	Driver   string `yaml:"driver,omitempty" json:"driver,omitempty"`
	External bool   `yaml:"external,omitempty" json:"external,omitempty"`
}

type ComposeDependentConfig struct {
	ServiceName string `yaml:"-"`
	Condition   string `json:"condition" yaml:"condition,omitempty"`
}

type ComposeHealthCheckTest []string

func (t ComposeHealthCheckTest) MarshalYAML() (any, error) {
	nodes := []*yaml.Node{}
	for _, item := range t {
		nodes = append(nodes, &yaml.Node{
			Value: item,
			Kind:  yaml.ScalarNode,
		})
	}
	return &yaml.Node{
		Kind:    yaml.SequenceNode,
		Style:   yaml.FlowStyle,
		Tag:     "!!seq",
		Content: nodes,
	}, nil
}

type ComposeHealthcheckConfig struct {
	Test        ComposeHealthCheckTest `json:"test,omitempty" yaml:"test,omitempty"`
	Timeout     string                 `yaml:"timeout,omitempty" json:"timeout,omitempty"`
	Interval    string                 `yaml:"interval,omitempty" json:"interval,omitempty"`
	Retries     *uint64                `yaml:"retries,omitempty" json:"retries,omitempty"`
	StartPeriod string                 `yaml:"start_period,omitempty" json:"start_period,omitempty"`
	Disable     bool                   `yaml:"disable,omitempty" json:"disable,omitempty"`
}

type ComposeDependsOnConfig map[string]*ComposeDependentConfig

func (d *ComposeDependsOnConfig) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind == yaml.SequenceNode {
		*d = make(map[string]*ComposeDependentConfig)
		for _, item := range node.Content {
			(*d)[item.Value] = &ComposeDependentConfig{
				ServiceName: item.Value,
			}
		}
		return nil
	}
	if node.Kind == yaml.MappingNode {
		*d = make(map[string]*ComposeDependentConfig)
		for i := 0; i < len(node.Content); i += 2 {
			serviceName := node.Content[i].Value
			dep := &ComposeDependentConfig{
				ServiceName: serviceName,
			}
			if node.Content[i+1].Kind == yaml.MappingNode {
				for _, conditionPair := range node.Content[i+1].Content {
					dep.Condition = conditionPair.Value
				}
			}

			(*d)[serviceName] = dep
		}
		return nil
	}

	return fmt.Errorf("invalid depends_on format")
}

func (d *ComposeDependsOnConfig) MarshalYAML() (any, error) {
	allSimple := true
	for _, dep := range *d {
		if dep.Condition != "" {
			allSimple = false
			break
		}
	}

	if allSimple {
		var services []string
		for service := range *d {
			services = append(services, service)
		}
		return services, nil
	}

	result := map[string]any{}
	for service, dep := range *d {
		if dep.Condition != "" {
			result[service] = map[string]string{
				"condition": dep.Condition,
			}
		} else {
			result[service] = nil
		}
	}
	return result, nil
}

type ComposeEnvironmentConfig map[string]string

func (e *ComposeEnvironmentConfig) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind == yaml.SequenceNode {
		*e = make(map[string]string)
		for _, item := range node.Content {
			match := regEnv.FindAllStringSubmatch(item.Value, -1)
			if len(match) > 0 {
				(*e)[match[0][1]] = match[0][2]
			} else {
				return fmt.Errorf("invalid environment format: %s", item.Value)
			}
		}
		return nil
	}
	if node.Kind == yaml.MappingNode {
		*e = make(map[string]string)
		for i := 0; i < len(node.Content); i += 2 {
			(*e)[node.Content[i].Value] = node.Content[i+1].Value
		}
		return nil
	}
	return fmt.Errorf("invalid environment format")
}

type ComposeServiceConfig struct {
	ServiceName   string                    `json:"-" yaml:"-"`
	Image         string                    `json:"image" yaml:"image"`
	ContainerName string                    `json:"container_name,omitempty" yaml:"container_name,omitempty"`
	Hostname      string                    `json:"hostname,omitempty" yaml:"hostname,omitempty"`
	Restart       string                    `json:"restart,omitempty" yaml:"restart,omitempty"`
	Environment   *ComposeEnvironmentConfig `json:"environment,omitempty" yaml:"environment,omitempty"`
	Logging       *types.LoggingConfig      `json:"logging,omitempty" yaml:"logging,omitempty"`
	Networks      []string                  `json:"networks,omitempty" yaml:"networks,omitempty"`
	Ports         []string                  `json:"ports,omitempty" yaml:"ports,omitempty"`
	Volumes       []string                  `json:"volumes,omitempty" yaml:"volumes,omitempty"`
	Labels        *types.Labels             `json:"labels,omitempty" yaml:"labels,omitempty"`
	DependsOn     *ComposeDependsOnConfig   `json:"depends_on,omitempty" yaml:"depends_on,omitempty"`
	Healthcheck   *ComposeHealthcheckConfig `json:"healthcheck,omitempty" yaml:"healthcheck,omitempty"`
	Privileged    bool                      `json:"privileged,omitempty" yaml:"privileged,omitempty"`
	SecurityOpt   []string                  `json:"security_opt,omitempty" yaml:"security_opt,omitempty"`
}

func (serviceConf *ComposeServiceConfig) GetVersion() string {
	match := regServiceImage.FindAllStringSubmatch(serviceConf.Image, -1)
	if len(match) > 0 {
		if len(match[0]) == 4 {
			return match[0][3]
		}
	}
	return "latest"
}

func (serviceConf *ComposeServiceConfig) SetVersion(version string) {
	serviceConf.Image = fmt.Sprintf("%s:%s", serviceConf.GetImageName(), version)
}

func (serviceConf *ComposeServiceConfig) GetImageName() string {
	match := regServiceImage.FindAllStringSubmatch(serviceConf.Image, -1)
	if len(match) > 0 {
		return match[0][1]
	}
	return ""
}

func (serviceConf *ComposeServiceConfig) GetGitRegistry() string {
	if serviceConf.Labels == nil {
		return ""
	}
	registry, ok := (*serviceConf.Labels)["git.repository"]
	if ok {
		return registry
	}
	return ""
}

/*type ComposeServicesConfig []*ComposeServiceConfig

func (servicesConf *ComposeServicesConfig) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind == yaml.MappingNode {
		*servicesConf = make([]*ComposeServiceConfig, 0)
		for i := 0; i < len(node.Content); i += 2 {
			serviceName := node.Content[i].Value
			serviceConf := &ComposeServiceConfig{}
			err := jsoniter.UnmarshalFromString(node.Content[i+1].Value, serviceConf)
			if err != nil {
				fmt.Println(err)
				fmt.Println(node.Content[i].Value, len(node.Content[i+1].Value))
				fmt.Println(node.Line, node.Column)
				return err
			}
			serviceConf.ServiceName = serviceName
			*servicesConf = append(*servicesConf, serviceConf)
		}
		return nil
	}
	return fmt.Errorf("invalid services format")
}

func (servicesConf *ComposeServicesConfig) MarshalYAML() (any, error) {
	result := map[string]any{}
	for _, serviceConf := range *servicesConf {
		result[serviceConf.ServiceName] = serviceConf
	}
	return result, nil
}*/

type ComposeServicesConfig map[string]*ComposeServiceConfig

func (servicesConf *ComposeServicesConfig) MarshalYAML() (any, error) {
	keys := make([]string, 0)
	for key, _ := range *servicesConf {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := map[string]any{}
	for _, key := range keys {
		result[key] = (*servicesConf)[key]
	}
	return result, nil
}

type ComposeConfig struct {
	Version  string                           `json:"version" yaml:"version"`
	Services *ComposeServicesConfig           `json:"services" yaml:"services"`
	Networks map[string]*ComposeNetworkConfig `json:"networks,omitempty" yaml:"networks,omitempty"`
	Volumes  map[string]types.VolumeConfig    `json:"volumes,omitempty" yaml:"volumes,omitempty"`
	Secrets  map[string]types.SecretConfig    `json:"secrets,omitempty" yaml:"secrets,omitempty"`
}

func (conf *ComposeConfig) ExportYAML() ([]byte, error) {
	return yaml.Marshal(conf)
}

func (conf *ComposeConfig) ExportJSON() ([]byte, error) {
	return jsoniter.Marshal(conf)
}

func (conf *ComposeConfig) GetService(name string) *ComposeServiceConfig {
	service, ok := (*conf.Services)[name]
	if ok {
		return service
	}
	return nil
	/*var service *ComposeServiceConfig
	for _, s := range *conf.Services {
		if s.ServiceName == name {
			service = s
			break
		}
	}
	return service*/
}

func (conf *ComposeConfig) SetService(name string, serviceConf *ComposeServiceConfig) {
	(*conf.Services)[name] = serviceConf
}

func GetConfigFromComposeFile(composeFilePath string) (*ComposeConfig, error) {
	ext := filepath.Ext(composeFilePath)
	content, err := os.ReadFile(composeFilePath)
	if err != nil {
		return nil, err
	}
	config := &ComposeConfig{}
	switch ext {
	case ".yml", ".yaml":
		err = yaml.Unmarshal(content, config)
	case ".json":
		err = jsoniter.Unmarshal(content, config)
	default:
		err = fmt.Errorf("unsupported compose file format: %s", ext)
	}
	if err != nil {
		return nil, err
	}
	return config, nil
}
