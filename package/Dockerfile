FROM registry.suse.com/bci/bci-base:15.5
RUN  zypper -n rm container-suseconnect && \
     zypper -n in qemu-tools
COPY package/qemu-wrapper.sh /usr/bin/qemu-wrapper.sh
COPY bin/vm-import-controller /usr/bin/vm-import-controller
USER 1000
CMD ["vm-import-controller"]
