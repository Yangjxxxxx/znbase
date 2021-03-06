  
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or
// implied. See the License for the specific language governing
// permissions and limitations under the License.

import { Location } from "history";
import { Action } from "redux";
import { ThunkAction } from "redux-thunk";
import { createSelector } from "reselect";

import { createPath } from "src/hacks/createPath";
import { userLogin, userLogout } from "src/util/api";
import { AdminUIState } from "src/redux/state";
import { LOGIN_PAGE, LOGOUT_PAGE } from "src/routes/login";
import { znbase } from "src/js/protos";
import { getDataFromServer } from "src/util/dataFromServer";

import UserLoginRequest = znbase.server.serverpb.UserLoginRequest;

const dataFromServer = getDataFromServer();

// State for application use.

export interface LoginState {
  useLogin(): boolean;
  loginEnabled(): boolean;
  hasAccess(): boolean;
  loggedInUser(): string;
}

class LoginEnabledState {
  apiState: LoginAPIState;

  constructor(state: LoginAPIState) {
    this.apiState = state;
  }

  useLogin(): boolean {
    return true;
  }

  loginEnabled(): boolean {
    return true;
  }

  hasAccess(): boolean {
    return this.apiState.loggedInUser != null;
  }

  loggedInUser(): string {
    return this.apiState.loggedInUser;
  }
}

class LoginDisabledState {
  useLogin(): boolean {
    return true;
  }

  loginEnabled(): boolean {
    return false;
  }

  hasAccess(): boolean {
    return true;
  }

  loggedInUser(): string {
    return null;
  }
}

class NoLoginState {
  useLogin(): boolean {
    return false;
  }

  loginEnabled(): boolean {
    return false;
  }

  hasAccess(): boolean {
    return true;
  }

  loggedInUser(): string {
    return null;
  }
}

// Selector

export const selectLoginState = createSelector(
  (state: AdminUIState) => state.login,
  (login: LoginAPIState) => {
    if (!dataFromServer.ExperimentalUseLogin) {
      return new NoLoginState();
    }

    if (!dataFromServer.LoginEnabled) {
      return new LoginDisabledState();
    }

    return new LoginEnabledState(login);
  },
);

function shouldRedirect(location: Location) {
  if (!location) {
    return false;
  }

  if (location.pathname === LOGOUT_PAGE) {
    return false;
  }

  return true;
}

export function getLoginPage(location: Location) {
  const query = !shouldRedirect(location) ? undefined : {
    redirectTo: createPath({
      pathname: location.pathname,
      search: location.search,
    }),
  };
  return {
    pathname: LOGIN_PAGE,
    query: query,
  };
}

// Redux implementation.

// State

export interface LoginAPIState {
  loggedInUser: string;
  error: Error;
  inProgress: boolean;
}

const emptyLoginState: LoginAPIState = {
  loggedInUser: dataFromServer.LoggedInUser,
  error: null,
  inProgress: false,
};

// Actions

const LOGIN_BEGIN = "znbaseui/auth/LOGIN_BEGIN";
const LOGIN_SUCCESS = "znbaseui/auth/LOGIN_SUCCESS";
const LOGIN_FAILURE = "znbaseui/auth/LOGIN_FAILURE";

const loginBeginAction = {
  type: LOGIN_BEGIN,
};

interface LoginSuccessAction extends Action {
  type: typeof LOGIN_SUCCESS;
  loggedInUser: string;
}

function loginSuccess(loggedInUser: string): LoginSuccessAction {
  return {
    type: LOGIN_SUCCESS,
    loggedInUser,
  };
}

interface LoginFailureAction extends Action {
  type: typeof LOGIN_FAILURE;
  error: Error;
}

function loginFailure(error: Error): LoginFailureAction {
  return {
    type: LOGIN_FAILURE,
    error,
  };
}

const LOGOUT_BEGIN = "znbaseui/auth/LOGOUT_BEGIN";

const logoutBeginAction = {
  type: LOGOUT_BEGIN,
};

export function doLogin(username: string, password: string): ThunkAction<Promise<void>, AdminUIState, void> {
  return (dispatch) => {
    dispatch(loginBeginAction);

    const loginReq = new UserLoginRequest({
      username,
      password,
    });
    return userLogin(loginReq)
      .then(
        () => { dispatch(loginSuccess(username)); },
        (err) => { dispatch(loginFailure(err)); },
      );
  };
}

export function doLogout(): ThunkAction<Promise<void>, AdminUIState, void> {
  return (dispatch) => {
    dispatch(logoutBeginAction);

    // Make request to log out, reloading the page whether it succeeds or not.
    // If there was a successful log out but the network dropped the response somehow,
    // you'll get the login page on reload. If The logout actually didn't work, you'll
    // be reloaded to the same page and can try to log out again.
    return userLogout()
      .then(
        () => {
          document.location.reload();
        },
        () => {
          document.location.reload();
        },
      );
  };
}

// Reducer

export function loginReducer(state = emptyLoginState, action: Action): LoginAPIState {
  switch (action.type) {
    case LOGIN_BEGIN:
      return {
        loggedInUser: null,
        error: null,
        inProgress: true,
      };
    case LOGIN_SUCCESS:
      return {
        loggedInUser: (action as LoginSuccessAction).loggedInUser,
        inProgress: false,
        error: null,
      };
    case LOGIN_FAILURE:
      return {
        loggedInUser: null,
        inProgress: false,
        error: (action as LoginFailureAction).error,
      };
    case LOGOUT_BEGIN:
      return {
        loggedInUser: state.loggedInUser,
        inProgress: true,
        error: null,
      };
    default:
      return state;
  }
}
