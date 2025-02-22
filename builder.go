package kustomizily

import (
	"bytes"
	"fmt"
	"sort"
)

type configMapConfig struct {
	name        string
	namespace   string
	labels      map[string]string
	annotations map[string]string
	immutable   bool
	files       []string
}

type secretConfig struct {
	name        string
	namespace   string
	labels      map[string]string
	annotations map[string]string
	immutable   bool
	files       []string
	typ         string
}

type dirConfig struct {
	resources  []string
	configMaps []configMapConfig
	secrets    []secretConfig
}

type kustomizationBuilder struct {
	buf *bytes.Buffer
}

func newKustomizationBuilder() *kustomizationBuilder {
	return &kustomizationBuilder{
		buf: bytes.NewBufferString("apiVersion: kustomize.config.k8s.io/v1beta1\nkind: Kustomization\n"),
	}
}

func (k *kustomizationBuilder) Build() []byte {
	return k.buf.Bytes()
}

func (k *kustomizationBuilder) WriteResources(resources []string) *kustomizationBuilder {
	if len(resources) == 0 {
		return k
	}

	k.buf.WriteString("\nresources:\n")
	sort.Strings(resources)
	for _, name := range resources {
		fmt.Fprintf(k.buf, "- %s\n", name)
	}
	return k
}

func (k *kustomizationBuilder) WriteConfigMapGenerator(configMaps []configMapConfig) *kustomizationBuilder {
	if len(configMaps) == 0 {
		return k
	}

	k.buf.WriteString("\nconfigMapGenerator:\n")
	for _, cm := range configMaps {
		fmt.Fprintf(k.buf, "- name: %s\n", cm.name)
		k.writeNamespace(cm.namespace)
		k.writeFiles("files", cm.files)

		k.buf.WriteString("  options:\n")
		k.buf.WriteString("    disableNameSuffixHash: true\n")
		k.writeMapFields("annotations", cm.annotations)
		k.writeMapFields("labels", cm.labels)
		k.writeBoolField("immutable", cm.immutable)
	}
	return k
}

func (k *kustomizationBuilder) WriteSecretGenerator(secrets []secretConfig) *kustomizationBuilder {
	if len(secrets) == 0 {
		return k
	}

	k.buf.WriteString("\nsecretGenerator:\n")
	for _, s := range secrets {
		fmt.Fprintf(k.buf, "- name: %s\n", s.name)
		k.writeNamespace(s.namespace)
		k.writeFiles("files", s.files)
		k.writeField("type", s.typ)

		k.buf.WriteString("  options:\n")
		k.buf.WriteString("    disableNameSuffixHash: true\n")
		k.writeMapFields("annotations", s.annotations)
		k.writeMapFields("labels", s.labels)
		k.writeBoolField("immutable", s.immutable)
	}
	return k
}

func (k *kustomizationBuilder) writeNamespace(namespace string) {
	if namespace != "" {
		fmt.Fprintf(k.buf, "  namespace: %s\n", namespace)
	}
}

func (k *kustomizationBuilder) writeFiles(fieldName string, files []string) {
	if len(files) > 0 {
		fmt.Fprintf(k.buf, "  %s:\n", fieldName)
		sort.Strings(files)
		for _, f := range files {
			fmt.Fprintf(k.buf, "  - %s\n", f)
		}
	}
}

func (k *kustomizationBuilder) writeField(name, value string) {
	if value != "" {
		fmt.Fprintf(k.buf, "  %s: %s\n", name, value)
	}
}

func (k *kustomizationBuilder) writeBoolField(name string, value bool) {
	if value {
		fmt.Fprintf(k.buf, "    %s: %t\n", name, value)
	}
}

func (k *kustomizationBuilder) writeMapFields(fieldName string, data map[string]string) {
	if len(data) > 0 {
		fmt.Fprintf(k.buf, "    %s:\n", fieldName)
		for key, value := range data {
			fmt.Fprintf(k.buf, "      %q: %q\n", key, value)
		}
	}
}
