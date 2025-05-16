/*
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package bootstrap

import (
	"bytes"
	_ "embed"
	"encoding/base64"
	"fmt"
	"net/url"
	"strings"
	"text/template"
)

type Ubuntu struct {
	Options
	ContainerRuntime string
}
type NodeBootstrapVariables struct {
	NeedsCgroupV2            bool
	ClusterEndpoint          string
	CABundle                 string
	BootstrapToken           string
	UserData                 *string
	PreInstallScript         *string
	KubeletConfigFile        string
	BootstrapKubeconfigFile  string
	ContainerRuntimeEndpoint string
	KubeConfigFile           string
	LogLevel                 string
}

var (
	//go:embed bootstrap-kubelet.conf.gtpl
	bootstrapKubeletText     string
	bootstrapKubeletTemplate = template.Must(template.New("bootstrapKubelet").Parse(bootstrapKubeletText))

	//go:embed  containerd.toml.gtpl
	containerdConfigTemplateText string
	containerdConfigTemplate     = template.Must(template.New("containerdconfig").Parse(containerdConfigTemplateText))

	//go:embed kubelet-config.json.gtpl
	kubeletConfigText         string
	kubeletConfigTextTemplate = template.Must(template.New("kubeletconfig").Parse(kubeletConfigText))

	//go:embed kubelet.service.gtpl
	kubeletServiceText         string
	kubeletServiceTextTemplate = template.Must(template.New("kubeletservice").Parse(kubeletServiceText))
)

var (
	staticNodeBootstrapVars = &NodeBootstrapVariables{
		NeedsCgroupV2:            true,
		KubeletConfigFile:        "/etc/kubernetes/kubelet-config.json",
		BootstrapKubeconfigFile:  "/etc/kubernetes/bootstrap-kubelet.conf",
		ContainerRuntimeEndpoint: "unix:///run/containerd/containerd.sock",
		KubeConfigFile:           "/etc/kubernetes/kubelet.conf",
		LogLevel:                 "2",
	}
)

func (c Ubuntu) Script() (string, error) {
	cbs, err := c.customBootstrapScript()
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString([]byte(cbs)), nil
}

func (c Ubuntu) customBootstrapScript() (string, error) {
	nbv := staticNodeBootstrapVars
	c.applyOptions(nbv)

	var caBundleArg string
	if c.CABundle != nil {
		caBundleArg = fmt.Sprintf("--kubelet-ca-cert '%s'", *c.CABundle)
	}
	var userData bytes.Buffer
	userData.WriteString("#!/bin/bash\n")
	userData.WriteString("mkdir -p \"/etc/self-k8s\"\n")
	userData.WriteString("mkdir -p \"/etc/kubernetes\"\n")
	if c.PreInstallScript != nil {
		if err := createKubeletInstall(&userData, nbv); err != nil {
			return "", err
		}
	}
	if err := createBootstrapScript(&userData, nbv); err != nil {
		return "", err
	}
	if err := createKubeletConfig(&userData, nbv); err != nil {
		return "", err
	}
	if err := createKubeletBootstrapConfig(&userData, nbv); err != nil {
		return "", err
	}
	if err := createKubeletService(&userData, nbv); err != nil {
		return "", err
	}
	if err := createContainerdConfig(&userData, nbv); err != nil {
		return "", err
	}

	url, _ := url.Parse(c.ClusterEndpoint)
	userData.WriteString(fmt.Sprintf("bash /etc/self-k8s/k8s-install.sh --apiserver-endpoint '%s' %s", url.Hostname(), caBundleArg))
	if args := c.kubeletExtraArgs(); len(args) > 0 {
		userData.WriteString(fmt.Sprintf(" \\\n--kubelet-extra-args '%s'", strings.Join(args, " ")))
	}

	if c.KubeletConfig != nil && len(c.KubeletConfig.ClusterDNS) > 0 {
		userData.WriteString(fmt.Sprintf(" \\\n--cluster-dns '%s'", c.KubeletConfig.ClusterDNS[0]))
	} else if c.ClusterDns != "" {
		userData.WriteString(fmt.Sprintf(" \\\n--cluster-dns '%s'", c.ClusterDns))
	}
	userData.WriteString(" \\\n> /etc/self-k8s/k8s-install.log\n")

	return userData.String(), nil
}

func (c Ubuntu) applyOptions(nbv *NodeBootstrapVariables) {
	nbv.ClusterEndpoint = c.ClusterEndpoint
	nbv.CABundle = *c.CABundle
	nbv.BootstrapToken = c.BootstrapToken
	nbv.UserData = c.CustomUserData
	nbv.PreInstallScript = c.PreInstallScript
}

func createKubeletInstall(userData *bytes.Buffer, nbv *NodeBootstrapVariables) error {
	userData.WriteString("cat << 'EOF' > /etc/self-k8s/kubelet-install.sh\n")
	userData.WriteString(*nbv.PreInstallScript)
	userData.WriteString("\nEOF\n")
	userData.WriteString("bash /etc/self-k8s/kubelet-install.sh > /etc/self-k8s/kubelet-install.log\n")
	return nil
}

func createBootstrapScript(userData *bytes.Buffer, nbv *NodeBootstrapVariables) error {
	userData.WriteString("cat << 'EOF' > /etc/self-k8s/k8s-install.sh\n")
	userData.WriteString(*nbv.UserData)
	userData.WriteString("\nEOF\n")
	return nil
}

func createKubeletConfig(userData *bytes.Buffer, nbv *NodeBootstrapVariables) error {
	// write bootstrap-kubelet.conf
	userData.WriteString("cat << 'EOF' > /etc/kubernetes/kubelet-config.json\n")

	var buffer bytes.Buffer
	if err := kubeletConfigTextTemplate.Execute(&buffer, *nbv); err != nil {
		return fmt.Errorf("error executing kubelet config template: %w", err)
	}
	userData.WriteString(buffer.String())
	userData.WriteString("\nEOF\n")
	return nil
}

func createKubeletService(userData *bytes.Buffer, nbv *NodeBootstrapVariables) error {
	userData.WriteString("cat << 'EOF' > /etc/systemd/system/kubelet.service\n")

	var buffer bytes.Buffer
	if err := kubeletServiceTextTemplate.Execute(&buffer, *nbv); err != nil {
		return fmt.Errorf("error executing kubelet.service template: %w", err)
	}
	userData.WriteString(buffer.String())
	userData.WriteString("\nEOF\n")
	return nil
}

func createKubeletBootstrapConfig(userData *bytes.Buffer, nbv *NodeBootstrapVariables) error {
	// write bootstrap-kubelet.conf
	userData.WriteString("cat << 'EOF' > /etc/kubernetes/bootstrap-kubelet.conf\n")
	var buffer bytes.Buffer
	if err := bootstrapKubeletTemplate.Execute(&buffer, *nbv); err != nil {
		return fmt.Errorf("error executing kubelet bootstrap config template: %w", err)
	}
	userData.WriteString(buffer.String())
	userData.WriteString("\nEOF\n")
	return nil
}

func createContainerdConfig(userData *bytes.Buffer, nbv *NodeBootstrapVariables) error {
	// write bootstrap-kubelet.conf
	userData.WriteString("cat << 'EOF' > /etc/containerd/config.toml\n")
	var buffer bytes.Buffer
	if err := containerdConfigTemplate.Execute(&buffer, *nbv); err != nil {
		return fmt.Errorf("error executing containerd config template: %w", err)
	}
	userData.WriteString(buffer.String())
	userData.WriteString("\nEOF\n")
	return nil
}
