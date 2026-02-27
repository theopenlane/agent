FROM cgr.dev/chainguard/bash:latest

USER 65532:65532

ARG NAME=openlane-agent

COPY ${NAME} /bin/openlane-agent

ENTRYPOINT ["/bin/openlane-agent"]
