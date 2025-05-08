FROM golang:latest AS builder

ADD go.mod /workspace/
ADD go.sum /workspace/
ADD cmd/blobserver /workspace/cmd/blobserver/

WORKDIR /workspace

RUN CGO_ENABLED=0 go build -o /blobserver ./cmd/blobserver

# TODO: Don't have permission to write to volumes if nonroot
#FROM gcr.io/distroless/static-debian12:nonroot
FROM gcr.io/distroless/static-debian12:latest

COPY --from=builder /blobserver /blobserver

ENTRYPOINT ["/blobserver"]
