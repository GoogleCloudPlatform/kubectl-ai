ARG BASE_IMAGE

FROM golang:latest AS builder

ADD go.mod /workspace/
ADD go.sum /workspace/
ADD cmd/llamacpp-frontend /workspace/cmd/llamacpp-frontend
WORKDIR /workspace/

RUN CGO_ENABLED=0 go build -o /llamacpp-frontend ./cmd/llamacpp-frontend

FROM ${BASE_IMAGE}

COPY --from=builder /llamacpp-frontend /llamacpp-frontend

ENTRYPOINT [ "/llamacpp-frontend" ]
