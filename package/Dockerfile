FROM alpine:3.15.0
RUN apk add qemu-img
COPY bin/vm-import-controller /usr/bin/vm-import-controller
USER 1000
CMD ["vm-import-controller"]
