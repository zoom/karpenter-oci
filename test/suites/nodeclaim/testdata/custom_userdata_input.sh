MIME-Version: 1.0
Content-Type: multipart/mixed; boundary="BOUNDARY"

--BOUNDARY
Content-Type: text/x-shellscript; charset="us-ascii"

#!/bin/bash -xe
bash /etc/oke/oke-install.sh --apiserver-endpoint '%s' --cluster-dns '%s' --kubelet-ca-cert '%s' \
--kubelet-extra-args '--register-with-taints=karpenter.sh/unregistered:NoExecute --node-labels="karpenter.sh/nodepool=karpenter-test" --max-pods=110 --system-reserved="memory=100Mi"' \

--BOUNDARY--
