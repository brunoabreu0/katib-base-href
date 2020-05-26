export const FILTER_JOBS = 'FILTER_JOBS';

export const filterJobs = (experimentName, experimentNamespace) => ({
  type: FILTER_JOBS,
  experimentName,
  experimentNamespace,
});

export const CHANGE_TYPE = 'CHANGE_TYPE';

export const changeType = (filter, checked) => ({
  type: CHANGE_TYPE,
  filter,
  checked,
});

export const FETCH_NAS_JOBS_REQUEST = 'FETCH_NAS_JOBS_REQUEST';
export const FETCH_NAS_JOBS_SUCCESS = 'FETCH_NAS_JOBS_SUCCESS';
export const FETCH_NAS_JOBS_FAILURE = 'FETCH_NAS_JOBS_FAILURE';

export const fetchNASJobs = () => ({
  type: FETCH_NAS_JOBS_REQUEST,
});

export const FETCH_NAS_JOB_INFO_REQUEST = 'FETCH_NAS_JOB_INFO_REQUEST';
export const FETCH_NAS_JOB_INFO_SUCCESS = 'FETCH_NAS_JOB_INFO_SUCCESS';
export const FETCH_NAS_JOB_INFO_FAILURE = 'FETCH_NAS_JOB_INFO_FAILURE';

export const fetchNASJobInfo = (experimentName, namespace) => ({
  type: FETCH_NAS_JOB_INFO_REQUEST,
  experimentName,
  namespace,
});
