FROM ghcr.io/prefix-dev/pixi:0.76.2-noble@sha256:8b206ef57005a902cb53f50dbaa47893a4038ca269f0b00038b51f18b1313cd4

WORKDIR /workspace

RUN pixi --version

CMD ["/bin/bash"]
