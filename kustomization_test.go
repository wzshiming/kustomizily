package kustomizily

import (
	"testing"
)

func TestLongestCommonPrefix(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		want string
	}{
		{"empty", nil, ""},
		{"single", []string{"abc"}, ""},
		{"no common", []string{"abc", "xyz"}, ""},
		{"common", []string{"app-config", "app-rules"}, "app-"},
		{"dash underscore equivalence", []string{"app_config", "app-config2"}, "app_config"},
		{"full shorter string", []string{"app", "app-x"}, "app"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := longestCommonPrefix(tt.in); got != tt.want {
				t.Errorf("longestCommonPrefix(%v) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestTrimPrefix(t *testing.T) {
	tests := []struct {
		s, prefix, want string
	}{
		{"app-config", "app-", "config"},
		{"app_config", "app-", "config"},
		{"xyz", "app-", "xyz"},
		{"ab", "abc", "ab"},
		{"demo-operator", "demo_", "operator"},
	}
	for _, tt := range tests {
		if got := trimPrefix(tt.s, tt.prefix); got != tt.want {
			t.Errorf("trimPrefix(%q, %q) = %q, want %q", tt.s, tt.prefix, got, tt.want)
		}
	}
}

func TestIndexOfSeparator(t *testing.T) {
	tests := []struct {
		s    string
		want int
	}{
		{"app-", 4},
		{"abc", -1},
		{"a_b-c", 4},
		{"", -1},
	}
	for _, tt := range tests {
		if got := indexOfSeparator(tt.s); got != tt.want {
			t.Errorf("indexOfSeparator(%q) = %d, want %d", tt.s, got, tt.want)
		}
	}
}

func TestGetShortName(t *testing.T) {
	tests := []struct {
		name string
		obj  k8sObject
		want string
	}{
		{
			"instance prefix trimmed",
			k8sObject{Metadata: metadata{Name: "myapp-controller", Labels: map[string]string{"app.kubernetes.io/instance": "myapp"}}},
			"controller",
		},
		{
			"colon replaced",
			k8sObject{Metadata: metadata{Name: "system:controller"}},
			"system_controller",
		},
		{
			"plain",
			k8sObject{Metadata: metadata{Name: "web"}},
			"web",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := getShortName(&tt.obj); got != tt.want {
				t.Errorf("getShortName() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGetTargetDir(t *testing.T) {
	tests := []struct {
		name string
		obj  k8sObject
		want string
	}{
		{
			"crd",
			k8sObject{APIVersion: "apiextensions.k8s.io/v1", Kind: "CustomResourceDefinition"},
			"crd",
		},
		{
			"component has priority",
			k8sObject{Metadata: metadata{Labels: map[string]string{"app.kubernetes.io/component": "ctl", "app": "x"}}},
			"ctl",
		},
		{
			"legacy component",
			k8sObject{Metadata: metadata{Labels: map[string]string{"component": "c2"}}},
			"c2",
		},
		{
			"name label",
			k8sObject{Metadata: metadata{Labels: map[string]string{"app.kubernetes.io/name": "n"}}},
			"n",
		},
		{
			"app label",
			k8sObject{Metadata: metadata{Labels: map[string]string{"app": "a"}}},
			"a",
		},
		{
			"no labels",
			k8sObject{},
			"",
		},
		{
			"unsafe traversal falls back to root",
			k8sObject{Metadata: metadata{Labels: map[string]string{"app": "../evil"}}},
			"",
		},
		{
			"unsafe dotdot falls back to root",
			k8sObject{Metadata: metadata{Labels: map[string]string{"app": ".."}}},
			"",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := getTargetDir(&tt.obj); got != tt.want {
				t.Errorf("getTargetDir() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSelectUniqueFilenameFuncForFiles(t *testing.T) {
	t.Run("common prefix trimmed", func(t *testing.T) {
		obj := &k8sObject{Kind: "ConfigMap", Metadata: metadata{Name: "cm"}}
		objs := []*filesObject{{k8sObject: obj, files: map[string][]byte{
			"app-config.yaml": nil,
			"app-rules.yaml":  nil,
		}}}
		uniq := map[string]struct{}{"kustomization.yaml": {}}
		fun := selectUniqueFilenameFuncForFiles(objs, uniq)
		if fun == nil {
			t.Fatal("no filename func selected")
		}
		if got := fun(obj, "app-config.yaml"); got != "config.yaml" {
			t.Errorf("got %q, want %q", got, "config.yaml")
		}
	})

	t.Run("duplicate keys fall back to name-qualified", func(t *testing.T) {
		cm1 := &k8sObject{Kind: "ConfigMap", Metadata: metadata{Name: "alpha"}}
		cm2 := &k8sObject{Kind: "ConfigMap", Metadata: metadata{Name: "beta"}}
		objs := []*filesObject{
			{k8sObject: cm1, files: map[string][]byte{"config": nil}},
			{k8sObject: cm2, files: map[string][]byte{"config": nil}},
		}
		uniq := map[string]struct{}{"kustomization.yaml": {}}
		fun := selectUniqueFilenameFuncForFiles(objs, uniq)
		if fun == nil {
			t.Fatal("no filename func selected")
		}
		if got := fun(cm1, "config"); got != "alpha_config" {
			t.Errorf("got %q, want %q", got, "alpha_config")
		}
	})
}

func TestSelectUniqueFilenameFuncForK8sObjects(t *testing.T) {
	t.Run("crd filename", func(t *testing.T) {
		crd := &k8sObject{
			APIVersion: "apiextensions.k8s.io/v1",
			Kind:       "CustomResourceDefinition",
			Metadata:   metadata{Name: "widgets.example.com"},
			Spec:       spec{Group: "example.com", Names: specNames{Plural: "widgets"}},
		}
		uniq := map[string]struct{}{"kustomization.yaml": {}}
		fun := selectUniqueFilenameFuncForK8sObjects([]*k8sObject{crd}, uniq)
		if fun == nil {
			t.Fatal("no filename func selected")
		}
		if got := fun(crd); got != "example.com_widgets.yaml" {
			t.Errorf("got %q, want %q", got, "example.com_widgets.yaml")
		}
	})

	t.Run("kind collision falls back to name", func(t *testing.T) {
		sa1 := &k8sObject{APIVersion: "v1", Kind: "ServiceAccount", Metadata: metadata{Name: "a"}}
		sa2 := &k8sObject{APIVersion: "v1", Kind: "ServiceAccount", Metadata: metadata{Name: "b"}}
		uniq := map[string]struct{}{"kustomization.yaml": {}}
		fun := selectUniqueFilenameFuncForK8sObjects([]*k8sObject{sa1, sa2}, uniq)
		if fun == nil {
			t.Fatal("no filename func selected")
		}
		if got := fun(sa1); got != "a.yaml" {
			t.Errorf("got %q, want %q", got, "a.yaml")
		}
	})
}
