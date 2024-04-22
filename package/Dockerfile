# syntax=docker/dockerfile:1.7.0

FROM registry.suse.com/bci/bci-base:15.5
RUN  zypper -n rm container-suseconnect && \
     zypper -n in qemu-tools

ARG TARGETPLATFORM

RUN if [ "$TARGETPLATFORM" != "linux/amd64" ] && [ "$TARGETPLATFORM" != "linux/arm64" ]; then \
     echo "Error: Unsupported TARGETPLATFORM: $TARGETPLATFORM" && \
     exit 1; \
     fi

ENV ARCH=${TARGETPLATFORM#linux/}

COPY package/qemu-wrapper.sh /usr/bin/qemu-wrapper.sh
COPY bin/vm-import-controller-${ARCH} /usr/bin/vm-import-controller
USER 1000
CMD ["vm-import-controller"]
