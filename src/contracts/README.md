# Service contracts

- `proto/`: Protocol Buffer definitions for internal RPCs selected for gRPC.
- `openapi/`: HTTP API specifications for the web application and external users.

Each interface has one authoritative definition. Generated clients are derived
from that definition, not maintained as parallel handwritten contracts.

No API definitions or generation pipeline exist yet. Generator versions,
configuration, and generated-code locations will be added with the first API.
This directory does not contain database access or business implementations.
