FROM gcr.io/distroless/static:nonroot
WORKDIR /
COPY --chown=0:0 ./bin/fabric /bin/

USER 65532:65532

ENTRYPOINT ["/bin/fabric"]
