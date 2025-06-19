MIME-Version: 1.0
Content-Type: multipart/mixed; boundary="BOUNDARY"

--BOUNDARY
Content-Type: text/x-shellscript; charset="us-ascii"

#!/bin/bash -xe
bash /etc/oke/oke-install.sh --apiserver-endpoint '%s' --cluster-dns '%s' --kubelet-ca-cert '%s' \
--kubelet-extra-args '--node-labels="testing/cluster=unspecified,custom-label=custom-value,custom-label2=custom-value2" --max-pods=110 --system-reserved="memory=100Mi"' \

--BOUNDARY--
