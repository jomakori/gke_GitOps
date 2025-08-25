package terratests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestKubeconfigExists validates that kubeconfig is accessible
func TestKubeconfigExists(t *testing.T) {
	t.Parallel()

	// Check if kubeconfig exists in standard locations
	kubeconfigPath := os.Getenv("KUBECONFIG")
	if kubeconfigPath == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			t.Skipf("Skipping test: %v", err)
		}
		kubeconfigPath = filepath.Join(homeDir, ".kube", "config")
	}

	// Check if kubeconfig file exists
	_, err := os.Stat(kubeconfigPath)
	if err != nil {
		t.Skipf("Skipping test: kubeconfig not found at %s", kubeconfigPath)
	}

	assert.FileExists(t, kubeconfigPath, "Kubeconfig should exist")
	t.Logf("Kubeconfig found at: %s", kubeconfigPath)
}

// TestManifestFilesExist validates that required manifest files exist
func TestManifestFilesExist(t *testing.T) {
	t.Parallel()

	requiredDirs := []string{
		"../../apps/argocd-appset/templates",
		"../../services/argocd-appset/templates",
	}

	for _, dir := range requiredDirs {
		t.Run(dir, func(t *testing.T) {
			_, err := os.Stat(dir)
			assert.NoError(t, err, "Directory %s should exist", dir)
			
			// Check if directory contains YAML files
			files, err := os.ReadDir(dir)
			assert.NoError(t, err)
			
			hasYamlFiles := false
			for _, file := range files {
				if filepath.Ext(file.Name()) == ".yaml" || filepath.Ext(file.Name()) == ".yml" {
					hasYamlFiles = true
					break
				}
			}
			
			assert.True(t, hasYamlFiles, "Directory %s should contain YAML files", dir)
		})
	}
}

// TestHelmChartsStructure validates basic Helm chart structure
func TestHelmChartsStructure(t *testing.T) {
	t.Parallel()

	chartDirs := []string{
		"../../apps/helm/demo-app",
		"../../apps/helm/notes-app",
	}

	for _, chartDir := range chartDirs {
		t.Run(chartDir, func(t *testing.T) {
			// Check Chart.yaml exists
			chartFile := filepath.Join(chartDir, "Chart.yaml")
			_, err := os.Stat(chartFile)
			assert.NoError(t, err, "Chart.yaml should exist in %s", chartDir)
			
			// Check values.yaml exists
			valuesFile := filepath.Join(chartDir, "values.yaml")
			_, err = os.Stat(valuesFile)
			assert.NoError(t, err, "values.yaml should exist in %s", chartDir)
			
			// Check templates directory exists
			templatesDir := filepath.Join(chartDir, "templates")
			_, err = os.Stat(templatesDir)
			assert.NoError(t, err, "templates directory should exist in %s", chartDir)
		})
	}
}

// TestServiceHelmCharts validates service Helm chart structure (may not have templates)
func TestServiceHelmCharts(t *testing.T) {
	t.Parallel()

	serviceChartDirs := []string{
		"../../services/helm/external-secrets",
		"../../services/helm/metrics-server",
	}

	for _, chartDir := range serviceChartDirs {
		t.Run(chartDir, func(t *testing.T) {
			// Check Chart.yaml exists
			chartFile := filepath.Join(chartDir, "Chart.yaml")
			_, err := os.Stat(chartFile)
			assert.NoError(t, err, "Chart.yaml should exist in %s", chartDir)
			
			// Check values.yaml exists
			valuesFile := filepath.Join(chartDir, "values.yaml")
			_, err = os.Stat(valuesFile)
			assert.NoError(t, err, "values.yaml should exist in %s", chartDir)
			
			// Service charts may not have templates if they use upstream charts
			templatesDir := filepath.Join(chartDir, "templates")
			_, err = os.Stat(templatesDir)
			if err != nil {
				t.Logf("Templates directory not required for service chart %s", chartDir)
			}
		})
	}
}

// TestArgoCDAppSetStructure validates ArgoCD ApplicationSet structure
func TestArgoCDAppSetStructure(t *testing.T) {
	t.Parallel()

	appSetDirs := []string{
		"../../apps/argocd-appset/templates",
		"../../services/argocd-appset/templates",
	}

	for _, appSetDir := range appSetDirs {
		t.Run(appSetDir, func(t *testing.T) {
			_, err := os.Stat(appSetDir)
			assert.NoError(t, err)
			
			// Check for YAML files in the directory
			files, err := os.ReadDir(appSetDir)
			assert.NoError(t, err)
			
			hasApplicationFiles := false
			for _, file := range files {
				if filepath.Ext(file.Name()) == ".yaml" || filepath.Ext(file.Name()) == ".yml" {
					content, err := os.ReadFile(filepath.Join(appSetDir, file.Name()))
					assert.NoError(t, err)
					
					contentStr := string(content)
					
					// If file contains template syntax, try to render it with dummy values
					if strings.Contains(contentStr, "{{") {
						// Comprehensive template replacement for testing
						rendered := strings.ReplaceAll(contentStr, "{{ .Values.repoUrl }}", "https://github.com/example/repo")
						rendered = strings.ReplaceAll(rendered, "{{ $.Values.repoUrl }}", "https://github.com/example/repo")
						rendered = strings.ReplaceAll(rendered, "{{ .Values.targetRevision }}", "main")
						rendered = strings.ReplaceAll(rendered, "{{ $.Values.targetRevision }}", "main")
						rendered = strings.ReplaceAll(rendered, `{{ .Values.argoNamespace | default "argocd" }}`, "argocd")
						rendered = strings.ReplaceAll(rendered, `{{ $.Values.argoNamespace | default "argocd" }}`, "argocd")
						rendered = strings.ReplaceAll(rendered, `{{ .Values.argoProject | default "default" }}`, "default")
						rendered = strings.ReplaceAll(rendered, `{{ $.Values.argoProject | default "default" }}`, "default")
						rendered = strings.ReplaceAll(rendered, `{{ .Values.destinationServer | default "https://kubernetes.default.svc" }}`, "https://kubernetes.default.svc")
						rendered = strings.ReplaceAll(rendered, `{{ $.Values.destinationServer | default "https://kubernetes.default.svc" }}`, "https://kubernetes.default.svc")
						
						// Handle MongoDB specific values
						rendered = strings.ReplaceAll(rendered, "{{ .Values.mongodb.enable }}", "true")
						rendered = strings.ReplaceAll(rendered, "{{ $.Values.mongodb.enable }}", "true")
						rendered = strings.ReplaceAll(rendered, "{{ .Values.mongodb }}", "true")
						rendered = strings.ReplaceAll(rendered, "{{ $.Values.mongodb }}", "true")
						rendered = strings.ReplaceAll(rendered, "{{ .Values.mongoDBCreds.user }}", "testuser")
						rendered = strings.ReplaceAll(rendered, "{{ $.Values.mongoDBCreds.user }}", "testuser")
						rendered = strings.ReplaceAll(rendered, "{{ .Values.mongoDBCreds.pw }}", "testpass")
						rendered = strings.ReplaceAll(rendered, "{{ $.Values.mongoDBCreds.pw }}", "testpass")
						rendered = strings.ReplaceAll(rendered, "{{ .Values.mongoDBCreds.host }}", "localhost")
						rendered = strings.ReplaceAll(rendered, "{{ $.Values.mongoDBCreds.host }}", "localhost")
						rendered = strings.ReplaceAll(rendered, "{{ .Values.storageClass }}", "standard")
						rendered = strings.ReplaceAll(rendered, "{{ $.Values.storageClass }}", "standard")
						
						// Handle conditional blocks by removing them
						rendered = strings.ReplaceAll(rendered, "{{- if and (.Values.mongodb) (.Values.mongodb.enable) -}}", "")
						rendered = strings.ReplaceAll(rendered, "{{- end -}}", "")
						rendered = strings.ReplaceAll(rendered, "{{- end }}", "")
						
						contentStr = rendered
					}
					
					// Check if it contains Application kind
					if strings.Contains(contentStr, "kind: Application") {
						hasApplicationFiles = true
						// Check for ArgoCD API version instead of annotations
						assert.Contains(t, contentStr, "argoproj.io/v1alpha1",
							"Application file should contain ArgoCD API version")
					}
				}
			}
			
			assert.True(t, hasApplicationFiles, "AppSet directory should contain Application files")
		})
	}
}

// Helper function to check if string contains substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && s[:len(substr)] == substr
}
