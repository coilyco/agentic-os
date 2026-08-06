# Forgejo storage measurement

`aosguard ops forgejo-storage measure` collects the application-aware storage
evidence needed after a general disk-pressure report identifies Forgejo as a
material owner.

## Authority boundary

The generic `aosguard ops kubectl` surface continues to deny `exec`. The
storage command is a separate sealed exec transport beside the Forgejo API
group. It accepts no caller arguments and fixes these details in embedded code:

* Namespace and application/database workload targets.
* Forgejo filesystem paths and bounded report depths.
* Every program and shell pipeline executed in the application workload.
* Every PostgreSQL statement executed in the database workload.

The command therefore cannot be widened into an arbitrary pod shell, path
reader, or SQL client by supplying another argument.

Specgen compiles the reviewed Python source into `aosguard`. At invocation it
materializes that source under a private temporary directory and gives Python
an absolute path. AOSguard never relies on a checkout-relative script path.

## Report

The command records the active Kubernetes context and PVC/pod ownership before
collecting independent sections for:

* Forgejo application root and managed data.
* Package and repository directory ownership.
* Largest Git packfiles.
* PostgreSQL database size.
* Referenced and unreferenced package blobs.
* Package ownership, largest packages, version ages, and cleanup rules.

Every section has a 120-second client timeout. A failed or timed-out section
does not suppress independent evidence from later sections. The command returns
nonzero when any section is incomplete and says which section failed on
standard error.

This is measurement only. It does not delete packages, run Git garbage
collection, truncate logs, recycle runners, or mutate Kubernetes resources.

## Layer boundary

Node-stats remains the general host and Kubernetes storage observer. It owns
root bytes and inodes, configured pressure paths, PVC attribution, and node
conditions without learning Forgejo's application schema. AOSguard owns this
application-specific permission surface and packaged bridge.

Infrastructure's attended measurement wrapper remains the rollout fallback
until the guarded command is present on the operator hosts. After rollout, that
wrapper can delegate to this command so the measurement procedure has one
owner.

## See also

* [aosguard](aosguard.md) - static operator policy and release boundary.
* [AOS and Ward boundary](ward-specs.md) - independent permission surfaces.
