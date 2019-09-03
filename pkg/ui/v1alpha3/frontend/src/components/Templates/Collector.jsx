import React from 'react';
import withStyles from '@material-ui/styles/withStyles';

import Typography from '@material-ui/core/Typography';
import Button from '@material-ui/core/Button';

import TemplateList from './Common/TemplateList';

import { connect } from 'react-redux';
import { openDialog, fetchCollectorTemplates } from '../../actions/templateActions';
import AddDialog from './Common/AddDialog';

const styles = theme => ({
    root: {
        flexGrow: 1,
        marginTop: 40,
    },
});


class Collector extends React.Component{
    
    componentDidMount() {
        this.props.fetchCollectorTemplates();
    }

    openAddDialog = () => {
        this.props.openDialog("add");
    }

    render() {

        const { classes } = this.props;

        const type = "collector";

        return (
            <div className={classes.root}>
                <Typography variant={"headline"} color={"primary"}>
                    Collector Manifests
                </Typography>
                <Button variant={"contained"} color={"primary"} onClick={this.openAddDialog}>
                    Add
                </Button>

                <TemplateList type={type} />
                <AddDialog type={type}/>
                
            </div>
        )
    }
}
export default connect(null, { openDialog, fetchCollectorTemplates })(withStyles(styles)(Collector));
