package kustomizily

import (
	"bytes"
	"fmt"
	"strings"
)

type filesObject struct {
	k8sObject *k8sObject
	files     map[string][]byte
}

type kustomizationBuilder struct {
	k8sObjects       []*k8sObject
	configMapObjects []*filesObject
	secretObjects    []*filesObject
	resources        []string
}

func newKustomizationBuilder() *kustomizationBuilder {
	return &kustomizationBuilder{}
}

func (k *kustomizationBuilder) AddK8sObject(obj *k8sObject) {
	k.k8sObjects = append(k.k8sObjects, obj)
}

func (k *kustomizationBuilder) AddConfigMapObjects(obj *filesObject) {
	k.configMapObjects = append(k.configMapObjects, obj)
}

func (k *kustomizationBuilder) AddSecretObjects(obj *filesObject) {
	k.secretObjects = append(k.secretObjects, obj)
}

func (k *kustomizationBuilder) AddResource(resource string) {
	k.resources = append(k.resources, resource)
}

func (k *kustomizationBuilder) Build(writeFile func(name string, data []byte) error) error {
	uniq := map[string]struct{}{
		"kustomization.yaml": {},
	}

	for _, resource := range k.resources {
		uniq[resource] = struct{}{}
	}

	k8sObjectFilenameFunc := selectUniqueFilenameFuncForK8sObjects(k.k8sObjects, uniq)
	if k8sObjectFilenameFunc == nil {
		return fmt.Errorf("no unique filename for k8s objects")
	}
	configMapObjectFilenameFunc := selectUniqueFilenameFuncForFiles(k.configMapObjects, uniq)
	if configMapObjectFilenameFunc == nil {
		return fmt.Errorf("no unique filename for config map objects")
	}
	secretObjectFilenameFunc := selectUniqueFilenameFuncForFiles(k.secretObjects, uniq)
	if secretObjectFilenameFunc == nil {
		return fmt.Errorf("no unique filename for secret objects")
	}

	buf := bytes.NewBufferString("apiVersion: kustomize.config.k8s.io/v1beta1\nkind: Kustomization\n")

	if err := k.writeResources(buf, k.resources, k.k8sObjects, k8sObjectFilenameFunc, writeFile); err != nil {
		return err
	}

	if err := k.writeGenerators(buf, "configMapGenerator", k.configMapObjects, configMapObjectFilenameFunc, writeFile); err != nil {
		return err
	}

	if err := k.writeGenerators(buf, "secretGenerator", k.secretObjects, secretObjectFilenameFunc, writeFile); err != nil {
		return err
	}

	return writeFile("kustomization.yaml", buf.Bytes())
}

func selectUniqueFilenameFuncForFiles(objects []*filesObject, uniq map[string]struct{}) func(obj *k8sObject, key string) string {
	funcs := []func(obj *k8sObject, key string) string{
		getGeneratorObjectShortFilenameByKey,
		getGeneratorObjectShortFilenameByKeyAndKind,
		getGeneratorObjectFilenameByKeyAndName,
		getGeneratorObjectFilenameFull,
	}
	for _, fun := range funcs {
		items, ok := isUniqueFilenameFunc(objects, uniq, fun)
		if !ok {
			continue
		}

		prefix := longestCommonPrefix(items)
		if prefix == "" {
			fillMap(uniq, items)
			return fun
		}

		index := indexOfSeparator(prefix)
		if index <= 0 {
			fillMap(uniq, items)
			return fun
		}

		newFunc := removeGeneratorObjectPrefix(fun, prefix[:index])
		newItems, ok := isUniqueFilenameFunc(objects, uniq, newFunc)
		if ok {
			fillMap(uniq, newItems)
			return newFunc
		}

		fillMap(uniq, items)
		return fun
	}
	return nil
}

func removeGeneratorObjectPrefix(fun func(obj *k8sObject, key string) string, prefix string) func(obj *k8sObject, key string) string {
	return func(obj *k8sObject, key string) string {
		return trimPrefix(fun(obj, key), prefix)
	}
}

func isUniqueFilenameFunc(objects []*filesObject, uniq map[string]struct{}, fun func(obj *k8sObject, key string) string) ([]string, bool) {
	items := []string{}
	localUniq := map[string]struct{}{}
	for _, obj := range objects {
		for key := range obj.files {
			name := fun(obj.k8sObject, key)
			if name == "" {
				return nil, false
			}
			if _, ok := uniq[name]; ok {
				return nil, false
			}
			if _, ok := localUniq[name]; ok {
				return nil, false
			}
			items = append(items, name)
			localUniq[name] = struct{}{}
		}
	}
	return items, true
}

func selectUniqueFilenameFuncForK8sObjects(objects []*k8sObject, uniq map[string]struct{}) func(obj *k8sObject) string {
	funcs := []func(obj *k8sObject) string{
		getCRDFilename,
		getK8sObjectShortFilenameByKind,
		getK8sObjectShortFilenameByName,
		getK8sObjectShortFilenameByNameAndKind,
		getK8sObjectFilenameFull,
	}
	for i, fun := range funcs {
		items, ok := isUniqueFilenameFuncForK8sObjects(objects, uniq, fun)
		if !ok {
			continue
		}

		if i == 0 {
			fillMap(uniq, items)
			return fun
		}

		prefix := longestCommonPrefix(items)
		if prefix == "" {
			fillMap(uniq, items)
			return fun
		}

		index := indexOfSeparator(prefix)
		if index <= 0 {
			fillMap(uniq, items)
			return fun
		}

		newFunc := removeK8sObjectsPrefix(fun, prefix[:index])
		newItems, ok := isUniqueFilenameFuncForK8sObjects(objects, uniq, newFunc)
		if ok {
			fillMap(uniq, newItems)
			return newFunc
		}

		fillMap(uniq, items)
		return fun
	}
	return nil
}

func removeK8sObjectsPrefix(fun func(obj *k8sObject) string, prefix string) func(obj *k8sObject) string {
	return func(obj *k8sObject) string {
		return trimPrefix(fun(obj), prefix)
	}
}

func fillMap(uniq map[string]struct{}, items []string) {
	for _, item := range items {
		uniq[item] = struct{}{}
	}
}

func isUniqueFilenameFuncForK8sObjects(objects []*k8sObject, uniq map[string]struct{}, fun func(obj *k8sObject) string) ([]string, bool) {
	items := []string{}
	localUniq := map[string]struct{}{}
	for _, obj := range objects {
		name := fun(obj)
		if name == "" {
			return nil, false
		}
		if _, ok := uniq[name]; ok {
			return nil, false
		}
		if _, ok := localUniq[name]; ok {
			return nil, false
		}
		items = append(items, name)
		localUniq[name] = struct{}{}
	}
	return items, true
}

func (k *kustomizationBuilder) writeResources(buf *bytes.Buffer, resources []string, objects []*k8sObject, filenameFunc func(obj *k8sObject) string, writeFile func(name string, data []byte) error) error {
	if len(resources) > 0 || len(objects) > 0 {
		buf.WriteString("\nresources:\n")
		for _, resource := range resources {
			fmt.Fprintf(buf, "- %s\n", resource)
		}
		for _, obj := range objects {
			name := filenameFunc(obj)
			if err := writeFile(name, obj.Raw); err != nil {
				return err
			}
			fmt.Fprintf(buf, "- %s\n", name)
		}
	}
	return nil
}

func (k *kustomizationBuilder) writeGenerators(buf *bytes.Buffer, generatorType string, objects []*filesObject, filenameFunc func(obj *k8sObject, key string) string, writeFile func(name string, data []byte) error) error {
	if len(objects) > 0 {
		buf.WriteString(fmt.Sprintf("\n%s:\n", generatorType))
		for _, obj := range objects {
			fmt.Fprintf(buf, "- name: %s\n", obj.k8sObject.Metadata.Name)
			if obj.k8sObject.Metadata.Namespace != "" {
				fmt.Fprintf(buf, "  namespace: %s\n", obj.k8sObject.Metadata.Namespace)
			}
			if generatorType == "secretGenerator" && obj.k8sObject.Type != "" {
				fmt.Fprintf(buf, "  type: %s\n", obj.k8sObject.Type)
			}
			buf.WriteString("  options:\n")
			buf.WriteString("    disableNameSuffixHash: true\n")
			k.writeMapFields(buf, "annotations", obj.k8sObject.Metadata.Annotations)
			k.writeMapFields(buf, "labels", obj.k8sObject.Metadata.Labels)
			if obj.k8sObject.Immutable {
				fmt.Fprintf(buf, "    immutable: true\n")
			}
			if err := k.writeFiles(buf, obj.files, filenameFunc, obj.k8sObject, writeFile); err != nil {
				return err
			}
		}
	}
	return nil
}

func (k *kustomizationBuilder) writeFiles(buf *bytes.Buffer, files map[string][]byte, filenameFunc func(obj *k8sObject, key string) string, k8sObj *k8sObject, writeFile func(name string, data []byte) error) error {
	buf.WriteString("  files:\n")
	for key, data := range files {
		name := filenameFunc(k8sObj, key)
		if err := writeFile(name, data); err != nil {
			return err
		}
		if name != key {
			fmt.Fprintf(buf, "  - %s=%s\n", key, name)
		} else {
			fmt.Fprintf(buf, "  - %s\n", key)
		}
	}
	return nil
}

func (k *kustomizationBuilder) writeMapFields(buf *bytes.Buffer, fieldName string, data map[string]string) {
	if len(data) > 0 {
		fmt.Fprintf(buf, "    %s:\n", fieldName)
		for key, value := range data {
			fmt.Fprintf(buf, "      %q: %q\n", key, value)
		}
	}
}

func getGeneratorObjectShortFilenameByKey(obj *k8sObject, key string) string {
	return key
}

func getGeneratorObjectShortFilenameByKeyAndKind(obj *k8sObject, key string) string {
	kind := strings.ToLower(obj.Kind)
	return fmt.Sprintf("%s_%s", kind, key)
}

func getGeneratorObjectFilenameByKeyAndName(obj *k8sObject, key string) string {
	return fmt.Sprintf("%s_%s", getShortName(obj), key)
}

func getGeneratorObjectFilenameFull(obj *k8sObject, key string) string {
	kind := strings.ToLower(obj.Kind)
	return fmt.Sprintf("%s_%s_%s", getShortName(obj), kind, key)
}

func getK8sObjectShortFilenameByKind(obj *k8sObject) string {
	kind := strings.ToLower(obj.Kind)
	return fmt.Sprintf("%s.yaml", kind)
}

func getK8sObjectShortFilenameByName(obj *k8sObject) string {
	return fmt.Sprintf("%s.yaml", getShortName(obj))
}

func getK8sObjectShortFilenameByNameAndKind(obj *k8sObject) string {
	return fmt.Sprintf("%s_%s.yaml", getShortName(obj), strings.ToLower(obj.Kind))
}

func getK8sObjectFilenameFull(obj *k8sObject) string {
	kind := strings.ToLower(obj.Kind)
	if !strings.Contains(obj.APIVersion, ".") && strings.HasSuffix(obj.APIVersion, "/v1") {
		kind = fmt.Sprintf("%s_%s", strings.TrimSuffix(obj.APIVersion, "/v1"), kind)
	} else if obj.APIVersion != "v1" {
		kind = fmt.Sprintf("%s_%s", strings.ReplaceAll(obj.APIVersion, "/", "_"), kind)
	}
	return fmt.Sprintf("%s_%s.yaml", getShortName(obj), kind)
}

func getCRDFilename(obj *k8sObject) string {
	if obj.Spec.Group == "" || obj.Spec.Names.Plural == "" {
		return ""
	}
	return fmt.Sprintf("%s_%s.yaml", obj.Spec.Group, obj.Spec.Names.Plural)
}

func getShortName(obj *k8sObject) string {
	name := obj.Metadata.Name
	instance := obj.Metadata.Labels["app.kubernetes.io/instance"]
	if instance != "" {
		name = trimPrefix(name, instance+"-")
	}
	name = strings.ReplaceAll(name, ":", "_")
	return name
}

func indexOfSeparator(s string) int {
	idx := strings.LastIndexAny(s, "-_")
	if idx == -1 {
		return -1
	}
	return idx + 1
}

func longestCommonPrefix(strs []string) string {
	if len(strs) == 0 {
		return ""
	}
	if len(strs) == 1 {
		return ""
	}

	minLen := len(strs[0])
	for _, s := range strs[1:] {
		if len(s) < minLen {
			minLen = len(s)
		}
	}

	for i := 0; i < minLen; i++ {
		char := strs[0][i]
		for _, s := range strs[1:] {
			if !charEqual(s[i], char) {
				return strs[0][:i]
			}
		}
	}

	return strs[0][:minLen]
}

func charEqual(a, b byte) bool {
	if a == b {
		return true
	}
	if a == '-' && b == '_' {
		return true
	}
	if a == '_' && b == '-' {
		return true
	}
	return false
}

func trimPrefix(s string, prefix string) string {
	if len(s) < len(prefix) {
		return s
	}
	l := len(prefix)
	for i := 0; i < l; i++ {
		if !charEqual(s[i], prefix[i]) {
			return s
		}
	}
	return s[l:]
}
