import Cookies from "universal-cookie";

const isLoggedIn = () => {
  const cookies = new Cookies();
  return (
    cookies.get("stash_csrf_v2") !== undefined ||
    cookies.get("session") !== undefined
  );
};

const SessionUtils = {
  isLoggedIn,
};

export default SessionUtils;
