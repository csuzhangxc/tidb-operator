FROM ghcr.io/pingcap-qe/bases/pingcap-base:v1.9.2
ARG TARGETARCH
ARG RCLONE_VERSION=v1.68.2
ARG SHUSH_VERSION=v1.5.5
ARG TOOLKIT_VERSION=v8.5.2
RUN dnf install -y ca-certificates bind-utils wget nc unzip && dnf clean all

RUN wget -nv https://github.com/ncw/rclone/releases/download/${RCLONE_VERSION}/rclone-${RCLONE_VERSION}-linux-${TARGETARCH}.zip \
  && unzip rclone-${RCLONE_VERSION}-linux-${TARGETARCH}.zip \
  && mv rclone-${RCLONE_VERSION}-linux-${TARGETARCH}/rclone /usr/local/bin \
  && chmod 755 /usr/local/bin/rclone \
  && rm -rf rclone-${RCLONE_VERSION}-linux-${TARGETARCH}.zip rclone-${RCLONE_VERSION}-linux-${TARGETARCH}

RUN wget -nv https://github.com/realestate-com-au/shush/releases/download/${SHUSH_VERSION}/shush_linux_${TARGETARCH} \
  && mv shush_linux_${TARGETARCH} /usr/local/bin/shush \
  && chmod 755 /usr/local/bin/shush

ARG TOOLKIT_NAME=${TOOLKIT_VERSION}-linux-${TARGETARCH}
ARG TOOLKIT_PKG_NAME=tidb-community-toolkit-${TOOLKIT_NAME}
RUN \
  wget -nv https://download.pingcap.org/${TOOLKIT_PKG_NAME}.tar.gz \
  && tar -xzf ${TOOLKIT_PKG_NAME}.tar.gz \
  && tar -xzf ${TOOLKIT_PKG_NAME}/tidb-lightning-${TOOLKIT_NAME}.tar.gz \
  && mv ${TOOLKIT_PKG_NAME}/tidb-lightning-ctl /tidb-lightning-ctl \
  && tar -xzf ${TOOLKIT_PKG_NAME}/dumpling-${TOOLKIT_NAME}.tar.gz \
  && chmod 755 /dumpling /tidb-lightning /tidb-lightning-ctl \
  && rm -rf ${TOOLKIT_PKG_NAME}.tar.gz \
  && rm -rf ${TOOLKIT_PKG_NAME}

COPY bin/${TARGETARCH}/tidb-backup-manager /tidb-backup-manager
COPY entrypoint.sh /entrypoint.sh


ENTRYPOINT ["/entrypoint.sh"]
