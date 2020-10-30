{
  global: {
    // User-defined global parameters; accessible to all component and environments, Ex:
    // replicas: 4,
  },
  components: {
    // Component-level parameters, defined initially from 'ks prototype use ...'
    // Each object below should correspond to a component in the components/ directory
    "workflows-v1beta1": {
      name: "some-very-very-very-very-very-long-name-katib-v1beta1-presubmit-test-374-6e32",
      namespace: "kubeflow-test-infra",
      registry: "gcr.io/kubeflow-ci",
      prow_env: "JOB_NAME=katib-v1beta1-presubmit-test,JOB_TYPE=presubmit,PULL_NUMBER=374,REPO_NAME=katib,REPO_OWNER=kubeflow,BUILD_NUMBER=6e32",
      versionTag: null,
    },
  },
}
