FROM --platform=linux/amd64 golang:1.23.6 as builder

WORKDIR /workspace

COPY . ./
RUN ls -l

RUN CGO_ENABLED=0 GO111MODULE=on GOOS=linux GOARCH=amd64 go build -trimpath -a -o karpenter ./cmd/controller/main.go

FROM alpine:3.19
ENV LC_AL en_US.utf8
WORKDIR /
COPY --from=builder /workspace/karpenter /usr/local/bin/karpenter

USER 65532:65532
ENTRYPOINT ["/usr/local/bin/karpenter"]