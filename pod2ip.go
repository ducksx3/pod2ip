package main

// note: compile this file using  "CGO_ENABLED=0 go build -tags netgo -a -v pod2ip.go"
import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubernetes "k8s.io/client-go/kubernetes"
	clientcmd "k8s.io/client-go/tools/clientcmd"
)

// the port the server listens on
const PORT = ":8585"

type NodeInfo struct {
	HostIP string `json:"HostIP"`
}

// initialize the cluster config
func connectToK8s() *kubernetes.Clientset {
	home, exists := os.LookupEnv("HOME")
	if !exists {
		home = "/root"
	}
	configPath := filepath.Join(home, ".kube", "config")
	config, err := clientcmd.BuildConfigFromFlags("", configPath)
	if err != nil {
		log.Fatalln("failed to create K8s config")
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Fatalln("Failed to create K8s clientset")
	}
	return clientset
}

// instantiate our client
var clientset = connectToK8s()

// start the HTTP server
func main() {
	handleRequests()
}

// query the Kubernetes API, retrieve the Pod info, and parse it for the HostIP
func queryPod(clientset *kubernetes.Clientset, podName string) (string, bool) {
	pod, err := clientset.CoreV1().Pods("default").Get(context.TODO(), podName, v1.GetOptions{})

	if errors.IsNotFound(err) {
		fmt.Printf("Pod %s not found in default namespace\n", podName)
	} else if statusError, isStatus := err.(*errors.StatusError); isStatus {
		fmt.Printf("Error getting pod %v\n", statusError.ErrStatus.Message)
	} else if err != nil {
		panic(err.Error())
	} else {
		fmt.Printf("Found %s pod in default namespace\n", podName)
		fmt.Println(pod.Status.HostIP)
		return pod.Status.HostIP, true
	}
	return "Error: Pod not found", false
}

// process any requests received and extract podName from the queries
func processRequest(w http.ResponseWriter, r *http.Request) {
	log.Printf("%s: %s\n", r.RemoteAddr, r.URL)
	w.Header().Set("Access-Control-Allow-Origin", "*")
	query := r.URL.Query()
	if len(query["podName"]) == 0 {
		return
	}
	podName := query["podName"][0]

	response, success := queryPod(clientset, podName)
	if !success {
		fmt.Fprintf(w, response)
		return
	}
	responseJSON := NodeInfo{
		HostIP: response,
	}
	json.NewEncoder(w).Encode(responseJSON)
}

// start the HTTP server
func handleRequests() {
	http.HandleFunc("/resolvePod/", processRequest)
	log.Printf("Now listening to podname API requests on %s\n", PORT)
	if err := http.ListenAndServe(PORT, nil); err != nil {
		log.Fatalf("unable to start server: %s", err.Error())
	}
}

// https://pliutau.com/rate-limit-http-requests/ - if we ever want to implement integrated rate-limiting

// Much of this code is sourced from a fantastic article by Narasimha Prasanna -
// https://dev.to/narasimha1997/create-kubernetes-jobs-in-golang-using-k8s-client-go-api-59ej
