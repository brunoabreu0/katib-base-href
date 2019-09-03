export const TOGGLE_MENU = "TOGGLE_MENU";

export const toggleMenu = (state) => {
    return {
        type: TOGGLE_MENU,
        state,
    };
};

export const CLOSE_SNACKBAR = "CLOSE_SNACKBAR";

export const closeSnackbar = () => {
    return {
        type: CLOSE_SNACKBAR,
    };
};

export const SUBMIT_YAML_REQUEST = "SUBMIT_YAML_REQUEST";
export const SUBMIT_YAML_FAILURE = "SUBMIT_YAML_FAILURE";
export const SUBMIT_YAML_SUCCESS = "SUBMIT_YAML_SUCCESS";

export const submitYaml = (yaml) => ({
    type: SUBMIT_YAML_REQUEST,
    yaml,
})

export const DELETE_EXPERIMENT_REQUEST = "DELETE_EXPERIMENT_REQUEST";
export const DELETE_EXPERIMENT_FAILURE = "DELETE_EXPERIMENT_FAILURE";
export const DELETE_EXPERIMENT_SUCCESS = "DELETE_EXPERIMENT_SUCCESS";

export const deleteExperiment = (experimentName) => ({
    type: DELETE_EXPERIMENT_REQUEST,
    experimentName,
})

export const OPEN_DELETE_EXPERIMENT_DIALOG = "OPEN_DELETE_EXPERIMENT_DIALOG";

export const openDeleteExperimentDialog = (experimentName) => ({
    type: OPEN_DELETE_EXPERIMENT_DIALOG,
    experimentName,
})

export const CLOSE_DELETE_EXPERIMENT_DIALOG = "CLOSE_DELETE_EXPERIMENT_DIALOG";

export const closeDeleteExperimentDialog = () => ({
    type: CLOSE_DELETE_EXPERIMENT_DIALOG,
})
