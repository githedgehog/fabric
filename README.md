# fabric

README isn't up-to-date.

// TODO(user): Add simple overview of use/purpose

## Building the Project

This project uses [just](https://github.com/casey/just) as a command
runner (similar to `make`). Follow these steps to build the project from
source.

### Prerequisites

- **Go** 1.21 or later
- **just** 1.36.0 or higher command runner - Install with:
  ```sh
  # On macOS or Linux (recommended for specific version)
  cargo install just --version 1.36.0

  # On macOS (using Homebrew - may install a different version)
  brew install just

  # Or download from https://github.com/casey/just/releases
  ```
### Build Steps

1. **Build all binaries**

   Run the main build command:
   ```sh
   just build
   ```

   This will:
   - Format and vet the Go code
   - Generate Kubernetes CRDs and RBAC manifests
   - Generate API documentation
   - Build all binaries for Linux/amd64:
     - `bin/fabric` - Main controller
     - `bin/agent` - Agent binary
     - `bin/hhfctl` - User-facing CLI tool
     - `bin/fabric-boot` - Boot service
     - `bin/fabric-dhcpd` - DHCP daemon

2. **View available commands**

   See all available build targets:
   ```sh
   just --list
   ```

### Other Useful Commands

- **Run tests**: `just test`
- **Run linters**: `just lint`
- **Build for multiple platforms**: `just build-multi`
- **Build Kubernetes artifacts**: `just kube-build`
- **Generate code/manifests**: `just gen`

## Description
// TODO(user): An in-depth paragraph about your project and overview of use

## Getting Started
You’ll need a Kubernetes cluster to run against. You can use [KIND](https://sigs.k8s.io/kind) to get a local cluster for testing, or run against a remote cluster.
**Note:** Your controller will automatically use the current context in your kubeconfig file (i.e. whatever cluster `kubectl cluster-info` shows).

### Running on the cluster
1. Install Instances of Custom Resources:

```sh
kubectl apply -f config/samples/
```

2. Build and push your image to the location specified by `IMG`:

```sh
make docker-build docker-push IMG=<some-registry>/fabric:tag
```

3. Deploy the controller to the cluster with the image specified by `IMG`:

```sh
make deploy IMG=<some-registry>/fabric:tag
```

### Uninstall CRDs
To delete the CRDs from the cluster:

```sh
make uninstall
```

### Undeploy controller
UnDeploy the controller from the cluster:

```sh
make undeploy
```

## Contributing
// TODO(user): Add detailed information on how you would like others to contribute to this project

### How it works
This project aims to follow the Kubernetes [Operator pattern](https://kubernetes.io/docs/concepts/extend-kubernetes/operator/).

It uses [Controllers](https://kubernetes.io/docs/concepts/architecture/controller/),
which provide a reconcile function responsible for synchronizing resources until the desired state is reached on the cluster.

### Test It Out
1. Install the CRDs into the cluster:

```sh
make install
```

2. Run your controller (this will run in the foreground, so switch to a new terminal if you want to leave it running):

```sh
make run
```

**NOTE:** You can also run this in one step by running: `make install run`

### Modifying the API definitions
If you are editing the API definitions, generate the manifests such as CRs or CRDs using:

```sh
make manifests
```

**NOTE:** Run `make --help` for more information on all potential `make` targets

More information can be found via the [Kubebuilder Documentation](https://book.kubebuilder.io/introduction.html)

## License

Copyright 2023 Hedgehog.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

