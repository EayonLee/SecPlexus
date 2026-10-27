# Shared libraries

Add a library here only when a concrete implementation is needed by multiple
applications. Keep application-specific code inside its owning application.

Each library uses its language's package and dependency conventions. Create
language groupings only when actual libraries need them; do not scaffold
generic platform, common, or utility packages in advance.

Interface definitions belong in `src/contracts/`. Shared libraries must not
import application-private implementations.
