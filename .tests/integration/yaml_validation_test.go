package integration

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/yaml"
)

// TestArgoCDManifestValidation tests that all ArgoCD manifests are valid YAML
// and contain required ArgoCD annotations
func TestArgoCDManifestValidation(t *testing.T) {
	t.Parallel()

	// Directories containing ArgoCD manifests
	manifestDirs := []string{
		"../../apps/argocd-appset",
		"../../services/argocd-appset",
		"../../apps/helm",
		"../../services/helm",
	}

	for _, dir := range manifestDirs {
		t.Run(fmt.Sprintf("Validate_%s", filepath.Base(dir)), func(t *testing.T) {
			err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
				if err != nil {
					return err
				}

				// Skip directories and non-YAML files
				if info.IsDir() {
					return nil
				}
				ext := filepath.Ext(path)
				if ext != ".yaml" && ext != ".yml" && ext != ".tpl" {
					return nil
				}

				// Skip Chart.yaml and values.yaml files for now
				if strings.HasSuffix(path, "Chart.yaml") || strings.HasSuffix(path, "values.yaml") {
					return nil
				}

				t.Run(filepath.Base(path), func(t *testing.T) {
					// Read the file content
					content, err := os.ReadFile(path)
					require.NoError(t, err, "Failed to read file %s", path)

					// Basic validation - file should not be empty
					assert.NotEmpty(t, content, "YAML file %s should not be empty", path)

					// Try to parse as YAML to ensure it's valid
					var yamlContent interface{}
					err = yaml.Unmarshal(content, &yamlContent)
					assert.NoError(t, err, "File %s should contain valid YAML", path)

					// For Application resources, check for required ArgoCD annotations
					if strings.Contains(string(content), "kind: Application") {
						assert.Contains(t, string(content), "argocd.argoproj.io",
							"Application resource in %s should contain ArgoCD annotations", path)
					}

					// For Helm templates, check for required fields
					if strings.Contains(path, "templates/") {
						checkHelmTemplateRequirements(t, string(content), path)
					}
				})

				return nil
			})

			assert.NoError(t, err, "Failed to walk directory %s", dir)
		})
	}
}

// TestHelmChartStructure validates basic Helm chart structure
func TestHelmChartStructure(t *testing.T) {
	t.Parallel()

	chartDirs := []string{
		"../../apps/helm",
		"../../services/helm",
	}

	for _, baseDir := range chartDirs {
		err := filepath.Walk(baseDir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			// Check for Chart.yaml files
			if info.Name() == "Chart.yaml" {
				t.Run(fmt.Sprintf("Chart_%s", filepath.Base(filepath.Dir(path))), func(t *testing.T) {
					content, err := os.ReadFile(path)
					require.NoError(t, err)

					var chart struct {
						APIVersion  string `yaml:"apiVersion"`
						Name        string `yaml:"name"`
						Version     string `yaml:"version"`
						Description string `yaml:"description"`
					}

					err = yaml.Unmarshal(content, &chart)
					assert.NoError(t, err, "Chart.yaml should be valid YAML")

					assert.Equal(t, "v2", chart.APIVersion, "Chart should use apiVersion v2")
					assert.NotEmpty(t, chart.Name, "Chart should have a name")
					assert.NotEmpty(t, chart.Version, "Chart should have a version")
					assert.NotEmpty(t, chart.Description, "Chart should have a description")
				})
			}

			return nil
		})

		assert.NoError(t, err, "Failed to walk directory %s", baseDir)
	}
}

// checkHelmTemplateRequirements validates basic requirements for Helm templates
func checkHelmTemplateRequirements(t *testing.T, content, path string) {
	// Check for resource limits in deployments
	if strings.Contains(content, "kind: Deployment") {
		assert.Contains(t, content, "resources:",
			"Deployment template %s should specify resource limits", path)
	}

	// Check for liveness/readiness probes in deployments
	if strings.Contains(content, "kind: Deployment") {
		assert.True(t,
			strings.Contains(content, "livenessProbe:") || strings.Contains(content, "readinessProbe:"),
			"Deployment template %s should have liveness or readiness probes", path)
	}

	// Check for service accounts in deployments
	if strings.Contains(content, "kind: Deployment") {
		assert.Contains(t, content, "serviceAccountName:",
			"Deployment template %s should specify serviceAccountName", path)
	}
}

// TestApplicationSetTemplates validates ArgoCD ApplicationSet template structure
func TestApplicationSetTemplates(t *testing.T) {
	t.Parallel()

	appSetDirs := []string{
		"../../apps/argocd-appset",
		"../../services/argocd-appset",
	}

	for _, dir := range appSetDirs {
		t.Run(fmt.Sprintf("AppSet_%s", filepath.Base(dir)), func(t *testing.T) {
			files, err := os.ReadDir(dir)
			require.NoError(t, err)

			hasApplicationFiles := false
			for _, file := range files {
				if filepath.Ext(file.Name()) == ".yaml" || filepath.Ext(file.Name()) == ".yml" {
					content, err := os.ReadFile(filepath.Join(dir, file.Name()))
					require.NoError(t, err)

					// Check if this is an Application resource
					if strings.Contains(string(content), "kind: Application") {
						hasApplicationFiles = true
						
						// Validate Application spec structure
						var app struct {
							Spec struct {
								Destination struct {
									Namespace string `yaml:"namespace"`
									Server    string `yaml:"server"`
								} `yaml:"destination"`
								Source struct {
									RepoURL string `yaml:"repoURL"`
									Path    string `yaml:"path"`
								} `yaml:"source"`
							} `yaml:"spec"`
						}

						err = yaml.Unmarshal(content, &app)
						assert.NoError(t, err, "Application file should be valid YAML")

						assert.NotEmpty(t, app.Spec.Destination.Namespace, 
							"Application should specify destination namespace")
						assert.NotEmpty(t, app.Spec.Destination.Server, 
							"Application should specify destination server")
						assert.NotEmpty(t, app.Spec.Source.RepoURL, 
							"Application should specify source repoURL")
						assert.NotEmpty(t, app.Spec.Source.Path, 
							"Application should specify source path")
					}
				}
			}

			assert.True(t, hasApplicationFiles, "Directory %s should contain Application files", dir)
		})
	}
}
