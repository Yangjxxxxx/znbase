
import _ from "lodash";
import React from "react";
import { connect } from "react-redux";
import { withRouter, WithRouterProps } from "react-router";
import { createSelector } from "reselect";

import { znbase } from "src/js/protos";
import { refreshNodes, refreshLiveness, refreshLocations } from "src/redux/apiReducers";
import { selectLocalityTree, LocalityTier, LocalityTree } from "src/redux/localities";
import { selectLocationsRequestStatus, selectLocationTree, LocationTree } from "src/redux/locations";
import {
  nodesSummarySelector,
  NodesSummary,
  selectNodeRequestStatus,
  selectLivenessRequestStatus,
  livenessStatusByNodeIDSelector,
  LivenessStatus,
  livenessByNodeIDSelector,
} from "src/redux/nodes";
import { AdminUIState } from "src/redux/state";
import { CLUSTERVIZ_ROOT } from "src/routes/visualization";
import { getLocality } from "src/util/localities";
import Loading from "src/views/shared/components/loading";
import { NodeCanvas } from "./nodeCanvas";

type Liveness = znbase.storage.Liveness;

interface NodeCanvasContainerProps {
  nodesSummary: NodesSummary;
  localityTree: LocalityTree;
  locationTree: LocationTree;
  livenessStatuses: { [id: string]: LivenessStatus };
  livenesses: { [id: string]: Liveness };
  dataExists: boolean;
  dataIsValid: boolean;
  dataErrors: Error[];
  refreshNodes: typeof refreshNodes;
  refreshLiveness: typeof refreshLiveness;
  refreshLocations: typeof refreshLocations;
}

export interface NodeCanvasContainerOwnProps {
  tiers: LocalityTier[];
}

class NodeCanvasContainer extends React.Component<NodeCanvasContainerProps & NodeCanvasContainerOwnProps & WithRouterProps> {
  componentWillMount() {
    this.props.refreshNodes();
    this.props.refreshLiveness();
    this.props.refreshLocations();
  }

  componentWillReceiveProps(props: NodeCanvasContainerProps & NodeCanvasContainerOwnProps & WithRouterProps) {
    props.refreshNodes();
    props.refreshLiveness();
    props.refreshLocations();
  }

  render() {
    const currentLocality = getLocality(this.props.localityTree, this.props.tiers);
    if (this.props.dataIsValid && _.isNil(currentLocality)) {
      this.props.router.replace(CLUSTERVIZ_ROOT);
    }

    return (
      <Loading
        loading={!this.props.dataExists}
        error={this.props.dataErrors}
        render={() => (
          <NodeCanvas
            localityTree={currentLocality}
            locationTree={this.props.locationTree}
            tiers={this.props.tiers}
            livenessStatuses={this.props.livenessStatuses}
            livenesses={this.props.livenesses}
          />
        )}
      />
    );
  }
}

const selectDataExists = createSelector(
  selectNodeRequestStatus,
  selectLocationsRequestStatus,
  selectLivenessRequestStatus,
  (nodes, locations, liveness) => !!nodes.data && !!locations.data && !!liveness.data,
);

const selectDataIsValid = createSelector(
  selectNodeRequestStatus,
  selectLocationsRequestStatus,
  selectLivenessRequestStatus,
  (nodes, locations, liveness) => nodes.valid && locations.valid && liveness.valid,
);

const dataErrors = createSelector(
  selectNodeRequestStatus,
  selectLocationsRequestStatus,
  selectLivenessRequestStatus,
  (nodes, locations, liveness) => [nodes.lastError, locations.lastError, liveness.lastError],
);

export default connect(
  (state: AdminUIState, _ownProps: NodeCanvasContainerOwnProps) => ({
    nodesSummary: nodesSummarySelector(state),
    localityTree: selectLocalityTree(state),
    locationTree: selectLocationTree(state),
    livenessStatuses: livenessStatusByNodeIDSelector(state),
    livenesses: livenessByNodeIDSelector(state),
    dataIsValid: selectDataIsValid(state),
    dataExists: selectDataExists(state),
    dataErrors: dataErrors(state),
  }),
  {
    refreshNodes,
    refreshLiveness,
    refreshLocations,
  },
)(withRouter(NodeCanvasContainer));
