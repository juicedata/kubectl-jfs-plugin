package debug

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

func Debug(clientSet *kubernetes.Clientset, ns, resourceType, resourceName string) error {
	var (
		out      string
		describe describeInterface
		err      error
	)

	switch resourceType {
	case "po":
		fallthrough
	case "pod":
		var pod *corev1.Pod
		if pod, err = clientSet.CoreV1().Pods(ns).Get(context.Background(), resourceName, metav1.GetOptions{}); err != nil {
			return err
		}
		describe, err = newPodDescribe(clientSet, pod)
		if err != nil {
			return err
		}
	case "pvc":
		var pvc *corev1.PersistentVolumeClaim
		if pvc, err = clientSet.CoreV1().PersistentVolumeClaims(ns).Get(context.Background(), resourceName, metav1.GetOptions{}); err != nil {
			return err
		}
		describe, err = newPVCDescribe(clientSet, pvc)
		if err != nil {
			return err
		}
	case "pv":
		var pv *corev1.PersistentVolume
		if pv, err = clientSet.CoreV1().PersistentVolumes().Get(context.Background(), resourceName, metav1.GetOptions{}); err != nil {
			return err
		}
		describe, err = newPVDescribe(clientSet, pv)
		if err != nil {
			return err
		}
	default:
		return fmt.Errorf("unsupported resource type: %s", resourceType)
	}

	out, err = describe.debug().describe()
	if err != nil {
		return err
	}
	fmt.Printf("%s\n", out)
	return nil
}

type describeInterface interface {
	failedf(reason string, args ...interface{})
	debug() describeInterface
	describe() (string, error)
}
