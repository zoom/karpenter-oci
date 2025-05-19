[Unit]
Description=Kubernetes Kubelet
After=containerd.service
Requires=containerd.service

[Service]
ExecStart=/usr/bin/kubelet \
--config {{ .KubeletConfigFile }} \
--bootstrap-kubeconfig {{ .BootstrapKubeconfigFile }} \
--container-runtime-endpoint {{ .ContainerRuntimeEndpoint }} \
--kubeconfig {{ .KubeConfigFile }} \
--v {{ .LogLevel }} \
$KUBELET_DEFAULT_ARGS $KUBELET_EXTRA_ARGS


Restart=always
# Configures the time to sleep before restarting a service. Restarts are rate-limited
# by default to 5 tries in 10s (see DefaultStartLimitInterval=10s and DefaultStartLimitBurst=5
# in /etc/systemd/system.conf.
RestartSec=10

[Install]
WantedBy=multi-user.target
