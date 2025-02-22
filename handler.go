package kustomizily

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"strings"

	"gopkg.in/yaml.v3"
)

// WriteFileFunc defines the signature for file writing functions used by the Handler
type WriteFileFunc func(dir string, name string, data []byte) error

// Handler processes multi-document YAML manifests, splitting them into individual resources
// and generating kustomization.yaml files for each directory. It calls writeFileFunc for
// each generated file (both resource files and kustomization files). Returns an error if
// any file operation fails or if YAML parsing fails.
type Handler struct {
	writeFile WriteFileFunc
	dirs      map[string]*dirConfig
}

// NewHandler creates a new Handler instance with the specified WriteFileFunc for file operations
func NewHandler(writeFile WriteFileFunc) *Handler {
	return &Handler{
		writeFile: writeFile,
		dirs:      map[string]*dirConfig{"": {}},
	}
}

// Process reads and processes multi-document YAML manifests from the provided reader.
// It splits resources into appropriate directories and handles special resource types.
func (h *Handler) Process(r io.Reader) error {
	scanner := newScanner(r)

	for scanner.Scan() {
		data := scanner.Bytes()
		data = bytes.TrimSpace(data)
		if len(data) == 0 {
			continue
		}

		obj, skip, err := h.parseYAMLObject(data)
		if err != nil {
			return err
		}
		if skip {
			continue
		}

		dir := getDir(&obj)
		h.ensureDirExists(dir)

		if err := h.handleResourceType(&obj, dir, data); err != nil {
			return err
		}
	}
	return nil
}

// Done finalizes the processing by generating kustomization.yaml files for all directories
func (h *Handler) Done() error {
	for dir, config := range h.dirs {
		data := newKustomizationBuilder().
			WriteResources(config.resources).
			WriteConfigMapGenerator(config.configMaps).
			WriteSecretGenerator(config.secrets).
			Build()
		if err := h.writeFile(dir, "kustomization.yaml", data); err != nil {
			return err
		}
	}
	return nil
}

func (h *Handler) parseYAMLObject(data []byte) (k8sObject, bool, error) {
	var obj k8sObject
	if err := yaml.Unmarshal(data, &obj); err != nil {
		return k8sObject{}, true, err
	}
	if obj.Kind == "" || obj.ApiVersion == "" || obj.Metadata.Name == "" {
		return k8sObject{}, true, nil
	}
	return obj, false, nil
}

func (h *Handler) ensureDirExists(dir string) {
	if _, ok := h.dirs[dir]; !ok {
		h.dirs[dir] = &dirConfig{}
		h.dirs[""].resources = append(h.dirs[""].resources, dir)
	}
}

func (h *Handler) handleResourceType(obj *k8sObject, dir string, data []byte) error {
	switch {
	case isCRD(obj):
		return h.handleCRD(obj, dir, data)
	case isConfigMap(obj):
		return h.handleConfigMap(obj, dir)
	case isSecret(obj):
		return h.handleSecret(obj, dir)
	default:
		return h.handleGenericResource(obj, dir, data)
	}
}

func isCRD(obj *k8sObject) bool {
	return obj.ApiVersion == "apiextensions.k8s.io/v1" && obj.Kind == "CustomResourceDefinition"
}

func (h *Handler) handleCRD(obj *k8sObject, dir string, data []byte) error {
	filename := getCRDFilename(obj)
	if err := h.writeFile(dir, filename, append(data, '\n')); err != nil {
		return err
	}
	h.dirs[dir].resources = append(h.dirs[dir].resources, filename)
	return nil
}

func isConfigMap(obj *k8sObject) bool {
	return obj.ApiVersion == "v1" && obj.Kind == "ConfigMap"
}

func (h *Handler) handleConfigMap(obj *k8sObject, dir string) error {
	files, err := h.processConfigMapData(obj, dir)
	if err != nil {
		return err
	}

	h.dirs[dir].configMaps = append(h.dirs[dir].configMaps, configMapConfig{
		name:        obj.Metadata.Name,
		namespace:   obj.Metadata.Namespace,
		labels:      obj.Metadata.Labels,
		annotations: obj.Metadata.Annotations,
		files:       files,
		immutable:   obj.Immutable,
	})
	return nil
}

func (h *Handler) processConfigMapData(obj *k8sObject, dir string) ([]string, error) {
	var files []string

	for key, value := range obj.Data {
		filename := getConfigMapFilename(obj, dir, key)
		if err := h.writeFile(dir, filename, []byte(value)); err != nil {
			return nil, err
		}
		files = append(files, fmt.Sprintf("%s=%s", key, filename))
	}

	for key, value := range obj.BinaryData {
		filename := getConfigMapFilename(obj, dir, key)
		data, err := base64.StdEncoding.DecodeString(value)
		if err != nil {
			return nil, err
		}
		if err := h.writeFile(dir, filename, data); err != nil {
			return nil, err
		}
		files = append(files, fmt.Sprintf("%s=%s", key, filename))
	}

	return files, nil
}

func isSecret(obj *k8sObject) bool {
	return obj.ApiVersion == "v1" && obj.Kind == "Secret"
}

func (h *Handler) handleSecret(obj *k8sObject, dir string) error {
	files, err := h.processSecretData(obj, dir)
	if err != nil {
		return err
	}

	h.dirs[dir].secrets = append(h.dirs[dir].secrets, secretConfig{
		name:        obj.Metadata.Name,
		namespace:   obj.Metadata.Namespace,
		labels:      obj.Metadata.Labels,
		annotations: obj.Metadata.Annotations,
		files:       files,
		immutable:   obj.Immutable,
		typ:         obj.Type,
	})
	return nil
}

func (h *Handler) processSecretData(obj *k8sObject, dir string) ([]string, error) {
	var files []string

	for key, value := range obj.Data {
		filename := getSecretFilename(obj, dir, key)
		data, err := base64.StdEncoding.DecodeString(value)
		if err != nil {
			return nil, err
		}
		if err := h.writeFile(dir, filename, data); err != nil {
			return nil, err
		}
		files = append(files, fmt.Sprintf("%s=%s", key, filename))
	}

	for key, value := range obj.StringData {
		filename := getSecretFilename(obj, dir, key)
		if err := h.writeFile(dir, filename, []byte(value)); err != nil {
			return nil, err
		}
		files = append(files, fmt.Sprintf("%s=%s", key, filename))
	}

	return files, nil
}

func (h *Handler) handleGenericResource(obj *k8sObject, dir string, data []byte) error {
	filename := getKindFilename(obj, dir)
	if err := h.writeFile(dir, filename, append(data, '\n')); err != nil {
		return err
	}
	h.dirs[dir].resources = append(h.dirs[dir].resources, filename)
	return nil
}

type metadata struct {
	Namespace   string            `yaml:"namespace"`
	Name        string            `yaml:"name"`
	Labels      map[string]string `yaml:"labels"`
	Annotations map[string]string `yaml:"annotations"`
}

type specNames struct {
	Plural string `yaml:"plural"`
}

type spec struct {
	// For CustomResourceDefinition
	Group string    `yaml:"group"`
	Names specNames `yaml:"names"`
}

type k8sObject struct {
	Kind       string   `yaml:"kind"`
	ApiVersion string   `yaml:"apiVersion"`
	Metadata   metadata `yaml:"metadata"`
	Spec       spec     `yaml:"spec"`

	// For ConfigMap and Secret
	Data       map[string]string `yaml:"data"`
	BinaryData map[string]string `yaml:"binaryData"`
	Immutable  bool              `yaml:"immutable"`
	Type       string            `yaml:"type"`
	StringData map[string]string `yaml:"stringData"`
}

func getDir(obj *k8sObject) string {
	if obj.ApiVersion == "apiextensions.k8s.io/v1" && obj.Kind == "CustomResourceDefinition" {
		return "crd"
	}
	dir := obj.Metadata.Labels["app.kubernetes.io/component"]
	if dir == "" {
		dir = obj.Metadata.Labels["component"]
	}
	if dir == "" {
		dir = obj.Metadata.Labels["app.kubernetes.io/name"]
	}
	if dir == "" {
		dir = obj.Metadata.Labels["app"]
	}
	return dir
}

func getShortName(obj *k8sObject) string {
	name := obj.Metadata.Name
	instance := obj.Metadata.Labels["app.kubernetes.io/instance"]
	if instance != "" {
		name = strings.TrimPrefix(name, instance+"-")
	}
	return name
}

func getCRDFilename(obj *k8sObject) string {
	return fmt.Sprintf("%s_%s.yaml", obj.Spec.Group, obj.Spec.Names.Plural)
}

func getConfigMapFilename(obj *k8sObject, dir, key string) string {
	if dir == obj.Metadata.Name {
		return fmt.Sprintf("configmap_%s", key)
	}
	return fmt.Sprintf("%s_configmap_%s", getShortName(obj), key)
}

func getSecretFilename(obj *k8sObject, dir, key string) string {
	if dir == obj.Metadata.Name {
		return fmt.Sprintf("secret_%s", key)
	}
	return fmt.Sprintf("%s_secret_%s", getShortName(obj), key)
}

func getKindFilename(obj *k8sObject, dir string) string {
	kind := strings.ToLower(obj.Kind)
	if !strings.Contains(obj.ApiVersion, ".") && strings.HasSuffix(obj.ApiVersion, "/v1") {
		kind = fmt.Sprintf("%s_%s", strings.TrimSuffix(obj.ApiVersion, "/v1"), kind)
	} else if obj.ApiVersion != "v1" {
		kind = fmt.Sprintf("%s_%s", strings.ReplaceAll(obj.ApiVersion, "/", "_"), kind)
	}
	if dir == obj.Metadata.Name {
		return fmt.Sprintf("%s.yaml", kind)
	}
	return fmt.Sprintf("%s_%s.yaml", getShortName(obj), kind)
}
