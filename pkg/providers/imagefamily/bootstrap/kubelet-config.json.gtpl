{
  "apiVersion": "kubelet.config.k8s.io/v1beta1",
  "authentication": {
    "anonymous": {
      "enabled": false
    },
    "x509": {
      "clientCAFile": "/etc/kubernetes/ca.crt"
    }
  },
  "systemReservedCgroup": "/system.slice",
  "kubeReservedCgroup": "/system.slice/kubelet.service",
  "cgroupDriver": "systemd",
  "clusterDomain": "cluster.local",
  "containerLogMaxFiles": 10,
  "containerLogMaxSize": "20Mi",
  "enableControllerAttachDetach": true,
  "eventRecordQPS": 50,
  "evictionPressureTransitionPeriod": "5m",
  "kind": "KubeletConfiguration",
  "protectKernelDefaults": false,
  "readOnlyPort": 0,
  "rotateCertificates": true,
  "runtimeRequestTimeout": "2m",
  "serializeImagePulls": false
}
