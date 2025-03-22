FROM golang:latest AS builder

WORKDIR /workspace

RUN git clone https://github.com/aquasecurity/trivy.git --branch=v0.60.0 --depth=1

WORKDIR /workspace/trivy

RUN go mod download

RUN cd cmd/trivy && CGO_ENABLED=0 go build -o /workspace/bin/trivy .

FROM gcr.io/distroless/static-debian12:latest

COPY --from=builder /workspace/bin/trivy /bin/trivy
COPY --from=builder /workspace/trivy/contrib/*.tpl /contrib/

ENTRYPOINT ["/bin/trivy"]
