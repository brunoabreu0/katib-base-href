import logging
from pkg.apis.manager.v1alpha3.python import api_pb2 as api

from pkg.suggestion.v1alpha3.internal.constant import *


logging.basicConfig(level=logging.DEBUG)
logger = logging.getLogger(__name__)


class HyperParameterSearchSpace(object):
    def __init__(self):
        self.goal = ""
        self.params = []

    @staticmethod
    def convert(experiment):
        search_space = HyperParameterSearchSpace()
        if experiment.spec.objective.type == api.MAXIMIZE:
            search_space.goal = MAX_GOAL
        elif experiment.spec.objective.type == api.MINIMIZE:
            search_space.goal = MIN_GOAL
        for p in experiment.spec.parameter_specs.parameters:
            search_space.params.append(
                HyperParameterSearchSpace.convertParameter(p))
        return search_space

    def __str__(self):
        return "HyperParameterSearchSpace(goal: {}, ".format(self.goal) + \
            "params: {})".format(", ".join([element.__str__() for element in self.params]))

    @staticmethod
    def convertParameter(p):
        if p.parameter_type == api.INT:
            # Default value for INT parameter step is 1
            step = 1
            if p.feasible_space.step is not None and p.feasible_space.step != "":
                step = p.feasible_space.step
            return HyperParameter.int(p.name, p.feasible_space.min, p.feasible_space.max, step)
        elif p.parameter_type == api.DOUBLE:
            return HyperParameter.double(p.name, p.feasible_space.min, p.feasible_space.max, p.feasible_space.step)
        elif p.parameter_type == api.CATEGORICAL:
            return HyperParameter.categorical(p.name, p.feasible_space.list)
        elif p.parameter_type == api.DISCRETE:
            return HyperParameter.discrete(p.name, p.feasible_space.list)
        else:
            logger.error(
                "Cannot get the type for the parameter: %s (%s)", p.name, p.parameter_type)


class HyperParameter(object):
    def __init__(self, name, type_, min_, max_, list_, step):
        self.name = name
        self.type = type_
        self.min = min_
        self.max = max_
        self.list = list_
        self.step = step

    def __str__(self):
        if self.type == INTEGER or self.type == DOUBLE:
            return "HyperParameter(name: {}, type: {}, min: {}, max: {}, step: {})".format(
                self.name, self.type, self.min, self.max, self.step)
        else:
            return "HyperParameter(name: {}, type: {}, list: {})".format(
                self.name, self.type, ", ".join(self.list))

    @staticmethod
    def int(name, min_, max_, step):
        return HyperParameter(name, INTEGER, min_, max_, [], step)

    @staticmethod
    def double(name, min_, max_, step):
        return HyperParameter(name, DOUBLE, min_, max_, [], step)

    @staticmethod
    def categorical(name, lst):
        return HyperParameter(name, CATEGORICAL, 0, 0, [str(e) for e in lst], 0)

    @staticmethod
    def discrete(name, lst):
        return HyperParameter(name, DISCRETE, 0, 0, [str(e) for e in lst], 0)
