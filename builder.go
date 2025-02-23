package kustomizily

import (
	"bytes"
	"encoding/base64"
	"io"
	"sort"

	"gopkg.in/yaml.v3"
)

// Builder processes multi-document YAML manifests, splitting them into individual resources
// and generating kustomization.yaml files for each directory. It calls writeFileFunc for
// each generated file (both resource files and kustomization files). Returns an error if
// any file operation fails or if YAML parsing fails.
type Builder struct {
	dirs map[string]*kustomizationBuilder
}

// NewBuilder creates a new Builder instance for handling kustomization operations
func NewBuilder() *Builder {
	return &Builder{
		dirs: map[string]*kustomizationBuilder{"": newKustomizationBuilder()},
	}
}

// Process reads and processes multi-document YAML manifests from the provided reader.
// It splits resources into appropriate directories and handles special resource types.
func (b *Builder) Process(r io.Reader) error {
	scanner := newScanner(r)

	for scanner.Scan() {
		data := scanner.Bytes()
		data = bytes.TrimSpace(data)
		if len(data) == 0 {
			continue
		}

		obj, skip, err := parseYAMLObject(data)
		if err != nil {
			return err
		}
		if skip {
			continue
		}

		obj.Raw = cloneBytes(data)

		if err := b.handleResourceType(&obj); err != nil {
			return err
		}
	}
	return nil
}

func parseYAMLObject(data []byte) (k8sObject, bool, error) {
	var obj k8sObject
	if err := yaml.Unmarshal(data, &obj); err != nil {
		return k8sObject{}, true, err
	}
	if obj.Kind == "" || obj.APIVersion == "" || obj.Metadata.Name == "" {
		return k8sObject{}, true, nil
	}
	return obj, false, nil
}

func cloneBytes(data []byte) []byte {
	clone := make([]byte, len(data))
	copy(clone, data)
	return clone
}

func (b *Builder) Build(writeFile func(dir string, name string, data []byte) error) error {
	sortedDirs := make([]string, 0, len(b.dirs))
	for dir := range b.dirs {
		sortedDirs = append(sortedDirs, dir)
	}
	sort.Strings(sortedDirs)

	for _, dir := range sortedDirs {
		err := b.dirs[dir].Build(func(name string, data []byte) error {
			return writeFile(dir, name, data)
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func (b *Builder) getKustomization(obj *k8sObject) *kustomizationBuilder {
	dir := getTargetDir(obj)
	if _, exists := b.dirs[dir]; !exists {
		b.dirs[dir] = newKustomizationBuilder()
		b.dirs[""].AddResource(dir)
	}
	return b.dirs[dir]
}

func getTargetDir(obj *k8sObject) string {
	if obj.APIVersion == "apiextensions.k8s.io/v1" && obj.Kind == "CustomResourceDefinition" {
		return "crd"
	}

	labels := obj.Metadata.Labels
	switch {
	case labels["app.kubernetes.io/component"] != "":
		return labels["app.kubernetes.io/component"]
	case labels["component"] != "":
		return labels["component"]
	case labels["app.kubernetes.io/name"] != "":
		return labels["app.kubernetes.io/name"]
	case labels["app"] != "":
		return labels["app"]
	default:
		return ""
	}
}

func (b *Builder) handleResourceType(obj *k8sObject) error {
	switch {
	case obj.APIVersion == "v1" && obj.Kind == "ConfigMap":
		return b.handleConfigMap(obj)
	case obj.APIVersion == "v1" && obj.Kind == "Secret":
		return b.handleSecret(obj)
	default:
		return b.handleGenericResource(obj)
	}
}

func (b *Builder) handleConfigMap(obj *k8sObject) error {
	fileGroup := &filesObject{
		k8sObject: obj,
		files:     make(map[string][]byte),
	}

	for key, value := range obj.Data {
		fileGroup.files[key] = []byte(value)
	}

	for key, value := range obj.BinaryData {
		data, err := base64.StdEncoding.DecodeString(value)
		if err != nil {
			return err
		}
		fileGroup.files[key] = data
	}

	b.getKustomization(obj).AddConfigMapObjects(fileGroup)
	return nil
}

func (b *Builder) handleSecret(obj *k8sObject) error {
	fileGroup := &filesObject{
		k8sObject: obj,
		files:     make(map[string][]byte),
	}

	for key, value := range obj.Data {
		data, err := base64.StdEncoding.DecodeString(value)
		if err != nil {
			return err
		}
		fileGroup.files[key] = data
	}

	for key, value := range obj.StringData {
		fileGroup.files[key] = []byte(value)
	}

	b.getKustomization(obj).AddSecretObjects(fileGroup)
	return nil
}

func (b *Builder) handleGenericResource(obj *k8sObject) error {
	b.getKustomization(obj).AddK8sObject(obj)
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
	APIVersion string   `yaml:"apiVersion"`
	Metadata   metadata `yaml:"metadata"`
	Spec       spec     `yaml:"spec"`

	// ConfigMap/Secret fields
	Data       map[string]string `yaml:"data"`
	BinaryData map[string]string `yaml:"binaryData"`
	StringData map[string]string `yaml:"stringData"`
	Immutable  bool              `yaml:"immutable"`
	Type       string            `yaml:"type"`

	Raw []byte
}
