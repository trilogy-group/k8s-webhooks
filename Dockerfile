FROM gcr.io/distroless/static
ADD webhooks-manager /
ENTRYPOINT ["/webhooks-manager"]
