package config

import (
	"context"
	// "encoding/json"
	"fmt"
	"io"

	// "log"
	"net/http"
	"os"
	"path/filepath"

	authv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog"
)

var kClientset *kubernetes.Clientset

func KubeClient() string {
	config, err, token := getClientConfig()
	if err != nil {
		klog.Fatal(err.Error())
	}
	kClientset, err = kubernetes.NewForConfig(config)
	if err != nil {
		klog.Fatal(err.Error())
	}
	return token

}

func getKubeConfigPath() string {
	defaultKubePath := filepath.Join(os.Getenv("HOME"), ".kube", "config")
	if _, err := os.Stat(defaultKubePath); os.IsNotExist(err) {
		// set default to empty string if path does not reslove
		defaultKubePath = ""
	}

	kubeConfig := getEnv("KUBECONFIG", defaultKubePath)
	return kubeConfig
}

func getClientConfig() (*rest.Config, error, string) {
	kubeConfigPath := getKubeConfigPath()
	var clientConfig *rest.Config
	var clientConfigError error

	if kubeConfigPath != "" {
		klog.Infof("Creating k8s client using KubeConfig at: %s", kubeConfigPath)
		clientConfig, clientConfigError = clientcmd.BuildConfigFromFlags("", kubeConfigPath)
	} else {
		klog.V(2).Info("Creating k8s client using InClusterClientConfig")
		clientConfig, clientConfigError = rest.InClusterConfig()
	}

	if clientConfigError != nil {
		klog.Fatal("Error getting Kube Config: ", clientConfigError)
	}

	token := clientConfig.BearerToken

	return clientConfig, clientConfigError, token
}

//verifies token (userid) with the TokenReview:
func Middleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

			var token string
			_, _, token = getClientConfig()
			// Read the value of the client identifier from the request header
			fmt.Fprintf(w, "%s %s %s \n", r.Method, r.URL, r.Proto)

			//Add the token from kubernetes to the request header
			r.Header.Add("Userid", token)

			//Iterate over all header fields
			for k, v := range r.Header {
				fmt.Fprintf(w, "header key %q, value %q\n", k, v)
			}
			//Retrieving and verifying the token:
			clientId := r.Header.Get("Userid")
			if len(clientId) == 0 {
				http.Error(w, "Could not find Userid", http.StatusUnauthorized)
				return
			}
			authenticated, err := verifyToken(clientId)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			if !authenticated {
				http.Error(w, "Invalid token", http.StatusForbidden)
				return
			}
			io.WriteString(w, "Authentication successful!")

		})
	}
}

func verifyToken(clientId string) (bool, error) {
	ctx := context.TODO()
	tr := authv1.TokenReview{
		Spec: authv1.TokenReviewSpec{
			Token: clientId,
		},
	}
	_, err := kClientset.AuthenticationV1().TokenReviews().Create(ctx, &tr, metav1.CreateOptions{})
	if err != nil {
		return false, err
	}
	return false, nil

}
