import React from 'react'
import { connect } from 'react-redux'
import { withStyles } from '@material-ui/core';
import Typography from '@material-ui/core/Typography';
import Button from '@material-ui/core/Button';
import { Link } from 'react-router-dom';
import LinearProgress from '@material-ui/core/LinearProgress';


import { fetchHPJobInfo } from '../../../actions/hpMonitorActions';

import HPJobPlot from './HPJobPlot';
import HPJobTable from './HPJobTable';
import TrialInfoDialog from './TrialInfoDialog';

const module = "hpMonitor";

const styles = theme => ({
    root: {
        width: '90%',
        margin: '0 auto',
        padding: 20,
    },
    loading: {
        marginTop: 30,
    },
    header: {
        marginTop: 30,
        textAlign: "center"
    }
})

class HPJobInfo extends React.Component {

    componentDidMount() {
        this.props.fetchHPJobInfo(this.props.match.params.name);
    }

    render () {
        const { classes } = this.props;
        return (
            <div className={classes.root}>
                <Link to="/katib/hp_monitor">
                    <Button variant={"contained"} color={"primary"}>
                        Back
                    </Button>
                </Link>
                {this.props.loading ? 
                <LinearProgress color={"primary"} className={classes.loading} />
                :
                <div>
                    <Typography  className = {classes.header} variant={"h5"}>
                        Experiment Name: {this.props.match.params.name}
                    </Typography>
                    <br />
                    <HPJobPlot name={this.props.match.params.name} />
                    <HPJobTable name={this.props.match.params.name} />
                    <TrialInfoDialog />
                </div>
                }
            </div>
        )
    }
}

const mapStateToProps = (state) => ({
  loading: state[module].loading,
})


export default connect(mapStateToProps, { fetchHPJobInfo })(withStyles(styles)(HPJobInfo));
