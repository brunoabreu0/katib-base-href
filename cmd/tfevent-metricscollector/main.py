import grpc
import argparse
import api_pb2
import api_pb2_grpc
from tfevent_loader import MetricsCollector
from logging import getLogger, StreamHandler, INFO

def parse_options():
    parser = argparse.ArgumentParser(
            description='TF-Event MetricsCollector',
            add_help = True
            )
    parser.add_argument("-m", "--manager_addr", type = str, default = "vizer-core")
    parser.add_argument("-p", "--manager_port", type = int, default = 6789 )
    parser.add_argument("-s", "--study_id", type = str, default = "")
    parser.add_argument("-w", "--worker_id", type = str, default = "")
    parser.add_argument("-d", "--log_dir", type = str, default = "/log")
    opt = parser.parse_args()
    return opt

if __name__ == '__main__':
    logger = getLogger(__name__)
    handler = StreamHandler()
    handler.setLevel(INFO)
    logger.setLevel(INFO)
    logger.addHandler(handler)
    logger.propagate = False
    opt = parse_options()
    mlset = api_pb2.MetricsLogSet(worker_id=opt.worker_id, metrics_logs=[])
    mc = MetricsCollector(opt.manager_addr, opt.manager_port, opt.study_id, opt.worker_id)
    mls = mc.parse_file(opt.log_dir)
    for ml in mls:
        mla = mlset.metrics_logs.add()
        mla.name = ml.name
        for v in ml.values:
            va = mla.values.add()
            va.time = v.time
            va.value = v.value
    channel = grpc.beta.implementations.insecure_channel(opt.manager_addr, opt.manager_port)
    with api_pb2.beta_create_Manager_stub(channel) as client:
        logger.info("In " + mlset.worker_id + " " + str(len(mlset.metrics_logs)) + " metrics will be reported.")
        client.ReportMetricsLogs(api_pb2.ReportMetricsLogsRequest(
            study_id=opt.study_id,
            metrics_log_sets=[mlset]
            ), 10)
