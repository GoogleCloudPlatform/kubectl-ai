ARG BUILDER_IMAGE
ARG BASE_IMAGE

FROM ${BUILDER_IMAGE} AS builder

ARG CMAKE_ARGS
ARG LLAMACPP_TAG

RUN apt-get update
RUN apt-get install -y g++ git cmake libcurl4-openssl-dev

WORKDIR /src
RUN git clone https://github.com/ggml-org/llama.cpp

WORKDIR /src/llama.cpp
RUN git checkout ${LLAMACPP_TAG}

RUN cmake -DCMAKE_BUILD_TYPE=Release ${CMAKE_ARGS} .
RUN cmake --build . -j16 --config Release --target llama-server --target rpc-server

RUN ldd /src/llama.cpp/bin/llama-server

FROM ${BASE_IMAGE} AS rpc-server

RUN apt-get update && apt-get install --yes libgomp1

COPY --from=builder /src/llama.cpp/bin/rpc-server /rpc-server
COPY --from=builder /src/llama.cpp/bin/lib*.so /lib/

ENTRYPOINT [ "/rpc-server" ]

FROM ${BASE_IMAGE} AS llamacpp-server

RUN apt-get update && apt-get install --yes libgomp1 libcurl4

COPY --from=builder /src/llama.cpp/bin/llama-server /llama-server
COPY --from=builder /src/llama.cpp/bin/lib*.so /lib/

ENTRYPOINT [ "/llama-server" ]
