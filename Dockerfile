# goreleaser bakes the binary outside this Dockerfile and injects it via
# COPY. Keep this image as small as possible: scratch + ca-certs only.
FROM gcr.io/distroless/static-debian12:nonroot

COPY arara /usr/local/bin/arara

USER nonroot:nonroot

ENTRYPOINT ["/usr/local/bin/arara"]
CMD ["--help"]
