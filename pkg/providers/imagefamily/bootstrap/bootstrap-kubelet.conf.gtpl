apiVersion: v1
clusters:
- cluster:
    certificate-authority-data: {{ .CABundle }}
    server: {{ .ClusterEndpoint }}
  name: bootstrap
contexts:
- context:
    cluster: bootstrap
    user: bootstrap
  name: bootstrap
current-context: bootstrap
kind: Config
preferences: {}
users:
- name: bootstrap
  user:
    token: {{ .BootstrapToken }}
