FROM ghcr.io/prefix-dev/pixi:0.82.1-noble@sha256:f83a5fc6d1bb9a2ced27faf2b26c04c5eb618eed22fe13eb60f9225db4817ce0

WORKDIR /workspace

RUN pixi --version

CMD ["/bin/bash"]
