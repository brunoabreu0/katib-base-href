import React from 'react';
import { connect } from 'react-redux';
import { withStyles } from '@material-ui/core/styles';
import List from '@material-ui/core/List';
import ListItem from '@material-ui/core/ListItem';
import ListItemIcon from '@material-ui/core/ListItemIcon';
import ListItemText from '@material-ui/core/ListItemText';
import { Link } from 'react-router-dom';
import { ListItemSecondaryAction, IconButton } from '@material-ui/core';

import { openDeleteExperimentDialog } from '../../../actions/generalActions';
import DeleteDialog from '../../Menu/DeleteDialog';

import ScheduleIcon from '@material-ui/icons/Schedule';
import RestoreIcon from '@material-ui/icons/Restore';
import HighlightOffIcon from '@material-ui/icons/HighlightOff';
import DoneIcon from '@material-ui/icons/Done';
import DeleteIcon from '@material-ui/icons/Delete';
import HourglassFullIcon from '@material-ui/icons/HourglassFull';

import { HP_MONITOR_MODULE } from '../../../constants/constants';

const styles = theme => ({
  created: {
    color: theme.colors.created,
  },
  running: {
    color: theme.colors.running,
  },
  restarting: {
    color: theme.colors.restarting,
  },
  succeeded: {
    color: theme.colors.succeeded,
  },
  failed: {
    color: theme.colors.failed,
  },
});

const HPJobList = props => {
  const { classes } = props;

  const onDeleteExperiment = (name, namespace) => event => {
    props.openDeleteExperimentDialog(name, namespace);
  };

  return (
    <div>
      <List component="nav">
        {props.filteredJobsList.map((job, i) => {
          let icon;
          if (job.status === 'Created') {
            icon = <HourglassFullIcon className={classes.created} />;
          } else if (job.status === 'Running') {
            icon = <ScheduleIcon className={classes.running} />;
          } else if (job.status === 'Restarting') {
            icon = <RestoreIcon className={classes.restarting} />;
          } else if (job.status === 'Succeeded') {
            icon = <DoneIcon className={classes.succeeded} />;
          } else if (job.status === 'Failed') {
            icon = <HighlightOffIcon className={classes.failed} />;
          }
          return (
            <ListItem
              button
              key={i}
              component={Link}
              to={`/katib/hp_monitor/${job.namespace}/${job.name}`}
            >
              <ListItemIcon>{icon}</ListItemIcon>
              <ListItemText inset primary={`${job.name}`} secondary={job.namespace} />
              <ListItemSecondaryAction>
                <IconButton
                  aria-label={'Delete'}
                  onClick={onDeleteExperiment(job.name, job.namespace)}
                >
                  <DeleteIcon />
                </IconButton>
              </ListItemSecondaryAction>
            </ListItem>
          );
        })}
      </List>
      <DeleteDialog />
    </div>
  );
};

const mapStateToProps = state => {
  return {
    filteredJobsList: state[HP_MONITOR_MODULE].filteredJobsList,
  };
};

export default connect(mapStateToProps, { openDeleteExperimentDialog })(
  withStyles(styles)(HPJobList),
);
