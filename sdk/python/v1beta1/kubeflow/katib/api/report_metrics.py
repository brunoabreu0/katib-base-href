# Copyright 2024 The Kubeflow Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#    http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

from datetime import datetime
from datetime import timezone
import os
from typing import Any, Dict

import grpc
from kubeflow.katib.constants import constants
import kubeflow.katib.katib_api_pb2 as katib_api_pb2
from kubeflow.katib.utils import utils


def report_metrics(
    metrics: Dict[str, Any],
    db_manager_address: str = constants.DEFAULT_DB_MANAGER_ADDRESS,
    timeout: int = constants.DEFAULT_TIMEOUT,
):
    """Push Metrics Directly to Katib DB

    Katib always passes Trial name as env variable `KATIB_TRIAL_NAME` to the training container.

    Args:
        metrics: Dict of metrics pushed to Katib DB.
            For examle, `metrics = {"loss": 0.01, "accuracy": 0.99}`.
        db-manager-address: Address for the Katib DB Manager in this format: `ip-address:port`.
        timeout: Optional, gRPC API Server timeout in seconds to report metrics.

    Raises:
        ValueError: The Trial name is not passed to environment variables.
        RuntimeError: Unable to push Trial metrics to Katib DB or
            metrics value has incorrect format (cannot be converted to type `float`).
    """

    # Get Trial's namespace and name
    namespace = utils.get_current_k8s_namespace()
    name = os.getenv("KATIB_TRIAL_NAME")
    if name is None:
        raise ValueError("The Trial name is not passed to environment variables")

    # Get channel for grpc call to db manager
    db_manager_address = db_manager_address.split(":")
    channel = grpc.beta.implementations.insecure_channel(
        db_manager_address[0], int(db_manager_address[1])
    )

    # Validate metrics value in dict
    for value in metrics.values():
        utils.validate_metrics_value(value)

    # Dial katib db manager to report metrics
    with katib_api_pb2.beta_create_DBManager_stub(channel) as client:
        try:
            timestamp = datetime.now(timezone.utc).strftime(constants.RFC3339_FORMAT)
            client.ReportObservationLog(
                request=katib_api_pb2.ReportObservationLogRequest(
                    trial_name=name,
                    observation_logs=katib_api_pb2.ObservationLog(
                        metric_logs=[
                            katib_api_pb2.MetricLog(
                                time_stamp=timestamp,
                                metric=katib_api_pb2.Metric(
                                    name=name, value=str(value)
                                ),
                            )
                            for name, value in metrics.items()
                        ]
                    ),
                ),
                timeout=timeout,
            )
        except Exception as e:
            raise RuntimeError(
                f"Unable to push metrics to Katib DB for Trial {namespace}/{name}. Exception: {e}"
            )
