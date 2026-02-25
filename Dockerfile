# Copyright 2026 Phillip Cloud
# Licensed under the Apache License, Version 2.0

FROM scratch
ARG TARGETPLATFORM
COPY $TARGETPLATFORM/micasa /bin/micasa
ENTRYPOINT ["/bin/micasa"]
