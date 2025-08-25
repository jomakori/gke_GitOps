package integration

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gruntwork-io/terratest/modules/k8s"
	"github.com/gruntwork-io/terratest/modules/retry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

// TestArgoCDApplicationDeployment tests that ArgoCD applications deploy successfully
func TestArgoCDApplicationDeployment(t *testing.T) {
	t.Parallel()

	// Get kubeconfig from environment or default location
	kubeconfigPath := os.Getenv("KUBECONFIG")
	if kubeconfigPath == "" {
		homeDir, err := os.UserHomeDir()
		require.NoError(t, err)
		kubeconfigPath = filepath.Join(homeDir, ".kube", "config")
	}

	// Create Kubernetes client
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	require.NoError(t, err)

	clientset, err := kubernetes.NewForConfig(config)
	require.NoError(t, err)

	// Test namespace for temporary deployments
	testNamespace := "gitops-integration-test"
	
	// Clean up any existing test namespace
	cleanupTestNamespace(t, clientset, testNamespace)
	
	// Create test namespace
	createTestNamespace(t, clientset, testNamespace)
	defer cleanupTestNamespace(t, clientset, testNamespace)

	// Test applications to verify
	applications := []struct {
		name      string
		namespace string
		chartPath string
	}{
		{
			name:      "demoapp",
			namespace: testNamespace,
			chartPath: "../../apps/helm/demo-app",
		},
		{
			name:      "notesapp", 
			namespace: testNamespace,
			chartPath: "../../apps/helm/notes-app",
		},
	}

	for _, app := range applications {
		t.Run(fmt.Sprintf("Deploy_%s", app.name), func(t *testing.T) {
			// Install the chart using Helm
			options := k8s.NewKubectlOptions("", kubeconfigPath, app.namespace)
			
			// Use helm install to deploy the application
			releaseName := fmt.Sprintf("test-%s", app.name)
			
			// Install with test values
			k8s.HelmInstall(t, options, releaseName, app.chartPath, map[string]string{
				"image.tag":                       "test",
				"replicaCount":                    "1",
				"resources.requests.memory":       "64Mi",
				"resources.requests.cpu":          "50m",
				"resources.limits.memory":         "128Mi", 
				"resources.limits.cpu":            "100m",
				"ingress.enabled":                 "false", // Disable ingress for tests
			})

			// Verify deployment becomes ready
			deploymentName := releaseName
			verifyDeploymentReady(t, clientset, app.namespace, deploymentName)

			// Verify service exists
			verifyServiceExists(t, clientset, app.namespace, deploymentName)

			// Clean up the release
			k8s.HelmDelete(t, options, releaseName, true)
		})
	}
}

// TestArgoCDAppSetTemplates tests that ArgoCD ApplicationSet templates are valid
func TestArgoCDAppSetTemplates(t *testing.T) {
	t.Parallel()

	appSetDirs := []string{
		"../../apps/argocd-appset",
		"../../services/argocd-appset",
	}

	for _, dir := range appSetDirs {
		t.Run(fmt.Sprintf("Validate_%s", filepath.Base(dir)), func(t *testing.T) {
			// Check that the directory exists and contains YAML files
			files, err := os.ReadDir(dir)
			require.NoError(t, err)

			hasYamlFiles := false
			for _, file := range files {
				if filepath.Ext(file.Name()) == ".yaml" || filepath.Ext(file.Name()) == ".yml" {
					hasYamlFiles = true
					
					// Read and validate the YAML content
					content, err := os.ReadFile(filepath.Join(dir, file.Name()))
					require.NoError(t, err)
					
					// Basic validation - file should not be empty
					assert.NotEmpty(t, content, "YAML file %s should not be empty", file.Name())
					
					// Check for required ArgoCD annotations if it's an Application resource
					if contains(string(content), "kind: Application") {
						assert.Contains(t, string(content), "argocd.argoproj.io", 
							"Application resource should contain ArgoCD annotations")
					}
				}
			}
			
			assert.True(t, hasYamlFiles, "Directory %s should contain YAML files", dir)
		})
	}
}

// Helper functions

func createTestNamespace(t *testing.T, clientset *kubernetes.Clientset, namespace string) {
	_, err := clientset.CoreV1().Namespaces().Create(context.Background(), &metav1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespace,
			Labels: map[string]string{
				"test": "gitops-integration",
			},
		},
	}, metav1.CreateOptions{})
	
	// Ignore error if namespace already exists
	if err != nil && !isAlreadyExistsError(err) {
		require.NoError(t, err)
	}
}

func cleanupTestNamespace(t *testing.T, clientset *kubernetes.Clientset, namespace string) {
	err := clientset.CoreV1().Namespaces().Delete(context.Background(), namespace, metav1.DeleteOptions{})
	// Ignore error if namespace doesn't exist
	if err != nil && !isNotFoundError(err) {
		t.Logf("Warning: Failed to delete namespace %s: %v", namespace, err)
	}
}

func verifyDeploymentReady(t *testing.T, clientset *kubernetes.Clientset, namespace, deploymentName string) {
	retry.DoWithRetry(t, "Wait for deployment to be ready", 10, 10*time.Second, func() (string, error) {
		deployment, err := clientset.AppsV1().Deployments(namespace).Get(context.Background(), deploymentName, metav1.GetOptions{})
		if err != nil {
			return "", fmt.Errorf("failed to get deployment: %v", err)
		}
		
		if deployment.Status.ReadyReplicas < 1 {
			return "", fmt.Errorf("deployment not ready yet. Ready: %d, Desired: %d", 
				deployment.Status.ReadyReplicas, *deployment.Spec.Replicas)
		}
		
		return "Deployment is ready", nil
	})
}

func verifyServiceExists(t *testing.T, clientset *kubernetes.Clientset, namespace, serviceName string) {
	_, err := clientset.CoreV1().Services(namespace).Get(context.Background(), serviceName, metav1.GetOptions{})
	assert.NoError(t, err, "Service %s should exist", serviceName)
}

func isAlreadyExistsError(err error) bool {
	return err != nil && contains(err.Error(), "AlreadyExists")
}

func isNotFoundError(err error) bool {
	return err != nil && contains(err.Error(), "NotFound")
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && s[:len(substr)] == substr
}
