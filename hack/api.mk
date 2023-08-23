.PHONY: lint-api
lint-api: kubevious ## Lint all APIs (K8s CRDs) and samples
	$(KUBEVIOUS) guard config/crd config/samples