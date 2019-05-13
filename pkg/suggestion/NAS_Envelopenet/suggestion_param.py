def parseSuggestionParam(params_raw):
    param_standard = {
        "gpus": ['categorical', list, []],
        "gpu_usage": ['value', float, [1e-6, 1.0]],
        "steps": ['value', int, [0, 'inf']],
        "batch_size": ['value', int, [1, 'inf']],
        "dataset": ['categorical', str, ["cifar10", "imagenet"]],
        "iterations": ['value', int, [0, 20]],
        "log_stats": ['categorical', bool, [True, False]],
        "data_dir":['categorical', str, ["data/"]],
        "max_layers_per_stage":['categorical', list, []]
    }

    suggestion_params = {
        "data_dir":"data/",
        "gpus": [],
        "gpu_usage": 0.47,
        "steps": 10000,
        "batch_size": 50,
        "dataset": "cifar10",
        "iterations": 5,
        "log_stats": True,
        "max_layers_per_stage":[7,6,3]
    }

    def checktype(param_name, param_value, check_mode, supposed_type, supposed_range=None):
        correct = True

        try:
            converted_value = supposed_type(param_value)
        except:
            correct = False
            print("Parameter {} is of wrong type. Set back to default value {}"
                  .format(param_name, suggestion_params[param_name]))

        if correct and check_mode == 'value':
            if not ((supposed_range[0] == '-inf' or converted_value >= supposed_range[0]) and
                    (supposed_range[1] == 'inf' or converted_value <= supposed_range[1])):
                correct = False
                print("Parameter {} out of range. Set back to default value {}"
                      .format(param_name, suggestion_params[param_name]))

        if correct:
            suggestion_params[param_name] = converted_value


    for param in params_raw:
        if param.name in suggestion_params.keys():
            checktype(param.name,
                      param.value,
                      param_standard[param.name][0],  # mode
                      param_standard[param.name][1],  # type
                      param_standard[param.name][2])  # range
        else:
            print("Unknown Parameter name: {}".format(param.name))

    return suggestion_params
