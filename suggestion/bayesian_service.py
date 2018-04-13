import random
import string

import grpc
import numpy as np

from api.python import api_pb2
from api.python import api_pb2_grpc
from suggestion.bayesianoptimization.src.bayesian_optimization_algorithm import BOAlgorithm
from suggestion.bayesianoptimization.src.algorithm_manager import AlgorithmManager


class BayesianService(api_pb2_grpc.SuggestionServicer):
    def __init__(self):
        # {
        #     study_id:[
        #         {
        #             trial_id:
        #             metric:
        #             parameters: []
        #         }
        #     ]
        # }
        self.trial_hist = {}
        # {
        #     study_id:{
        #         N:
        #     }
        # }
        self.service_params = {}

    def GenerateTrials(self, request, context):
        if request.study_id not in self.trial_hist.keys():
            self.trial_hist[request.study_id] = []
        X_train = []
        y_train = []

        for x in request.completed_trials:
            for trial in self.trial_hist[x.study_id]:
                if trial["trial_id"] == x.trial_id:
                    trial["metric"] = x.objective_value

        for x in self.trial_hist[request.study_id]:
            if x["metric"] is not None:
                X_train.append(x["parameters"])
                y_train.append(x["metric"])

        algo_manager = AlgorithmManager(
            study_id=request.study_id,
            study_config=request.configs,
            X_train=X_train,
            y_train=y_train,
        )

        lowerbound = np.array(algo_manager.lower_bound)
        upperbound = np.array(algo_manager.upper_bound)
        # print("lowerbound", lowerbound)
        # print("upperbound", upperbound)
        alg = BOAlgorithm(
            dim=algo_manager.dim,
            N=int(self.service_params[request.study_id]["N"]),
            lowerbound=lowerbound,
            upperbound=upperbound,
            X_train=algo_manager.X_train,
            y_train=algo_manager.y_train,
            mode=self.service_params[request.study_id]["mode"],
            trade_off=self.service_params[request.study_id]["trade_off"],
            # todo: support length_scale with array type
            length_scale=self.service_params[request.study_id]["length_scale"],
            noise=self.service_params[request.study_id]["noise"],
            nu=self.service_params[request.study_id]["nu"],
            kernel_type=self.service_params[request.study_id]["kernel_type"]
        )
        x_next = alg.get_suggestion().squeeze()

        # todo: maybe there is a better way to generate a trial_id
        trial_id = ''.join(random.sample(string.ascii_letters + string.digits, 12))
        self.trial_hist[request.study_id].append(dict({
            "trial_id": trial_id,
            "parameters": x_next,
            "metric": None,
        }))
        # print(x_next)

        x_next = algo_manager.parse_x_next(x_next)
        x_next = algo_manager.convert_to_dict(x_next)
        trial = api_pb2.Trial(
            trial_id=trial_id,
            study_id=request.study_id,
            parameter_set=[
                api_pb2.Parameter(
                    name=x["name"],
                    value=str(x["value"]),
                    parameter_type=x["type"],
                ) for x in x_next
            ],
            status=api_pb2.PENDING,
            eval_logs=[],
        )
        # print(self.trial_hist)

        return api_pb2.GenerateTrialsReply(
            trials=[trial],
            completed=False,
        )

    def SetSuggestionParameters(self, request, context):
        if request.study_id not in self.service_params.keys():
            self.service_params[request.study_id] = {
                "N": None,
                "length_scale": None,
                "noise": None,
                "nu": None,
                "kernel_type": None,
                "mode": None,
                "trade_off": None
            }
        for param in request.suggestion_parameters:
            if param.name not in self.service_params[request.study_id].keys():
                context.set_code(grpc.StatusCode.UNKNOWN)
                context.set_details("unknown suggestion parameter: "+param.name)
                return api_pb2.SetSuggestionParametersReply()
            if param.name == "length_scale" or param.name == "noise" or param.name == "nu" or param.name == "trade_off":
                self.service_params[request.study_id][param.name] = float(param.value)
            elif param.name == "N":
                self.service_params[request.study_id][param.name] = int(param.value)
            elif param.name == "kernel_type":
                if param.value != "rbf" and param.value != "matern":
                    context.set_code(grpc.StatusCode.UNKNOWN)
                    context.set_details("unknown kernel type: " + param.value)
                self.service_params[request.study_id][param.name] = param.value
            elif param.name == "mode":
                if param.value != "lcb" and param.value != "ei" and param.value != "pi":
                    context.set_code(grpc.StatusCode.UNKNOWN)
                    context.set_details("unknown acquisition mode: " + param.name)
                self.service_params[request.study_id][param.name] = param.value

        return api_pb2.SetSuggestionParametersReply()

    def StopSuggestion(self, request, context):
        if request.study_id in self.service_params.keys():
            del self.service_params[request.study_id]
            del self.trial_hist[request.study_id]
        return api_pb2.StopStudyReply()
