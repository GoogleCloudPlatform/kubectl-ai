ARG GO_VERSION=1.24.3

FROM golang:${GO_VERSION}-bookworm as builder

WORKDIR /src
COPY go.mod go.sum ./
COPY gollm/ ./gollm/
RUN go mod download

COPY cmd/ ./cmd/
COPY pkg/ ./pkg/

RUN CGO_ENABLED=0 go build -o kubectl-ai ./cmd/

FROM debian:bookworm-slim AS kubectl-tool
ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get update && apt-get install -y --no-install-recommends curl ca-certificates && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/*
RUN mkdir -p /opt/tools/kubectl/bin/
WORKDIR /opt/tools/kubectl
RUN curl -L -v https://dl.k8s.io/release/v1.33.0/bin/linux/amd64/kubectl -o /opt/tools/kubectl/bin/kubectl
RUN chmod +x /opt/tools/kubectl/bin/kubectl

FROM gcr.io/google.com/cloudsdktool/google-cloud-cli:slim as runtime

RUN apt-get update && \
    apt-get install -y --no-install-recommends google-cloud-cli-gke-gcloud-auth-plugin && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/*

COPY --from=builder /src/kubectl-ai /bin/kubectl-ai
COPY --from=kubectl-tool /opt/tools/kubectl/ /opt/tools/kubectl/

RUN ln -sf /opt/tools/kubectl/bin/kubectl /bin/kubectl

ENTRYPOINT [ "/bin/kubectl-ai" ]