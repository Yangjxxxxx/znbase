  

import React from "react";
import { withRouter, WithRouterProps } from "react-router";
import { connect } from "react-redux";

import { AdminUIState } from "src/redux/state";
import { selectLoginState, LoginState, getLoginPage } from "src/redux/login";

interface RequireLoginProps {
  loginState: LoginState;
}

class RequireLogin extends React.Component<WithRouterProps & RequireLoginProps> {
  componentWillMount() {
    this.checkLogin();
  }

  componentWillReceiveProps() {
    this.checkLogin();
  }

  checkLogin() {
    const { location, router } = this.props;

    if (!this.hasAccess()) {
      router.push(getLoginPage(location));
    }
  }

  hasAccess() {
    return this.props.loginState.hasAccess();
  }

  render() {
    if (!this.hasAccess()) {
      return null;
    }

    return (<React.Fragment>{ this.props.children }</React.Fragment>);
  }
}

// tslint:disable-next-line:variable-name
const RequireLoginConnected = connect(
  (state: AdminUIState) => {
    return {
      loginState: selectLoginState(state),
    };
  },
)(withRouter(RequireLogin));

export default RequireLoginConnected;
