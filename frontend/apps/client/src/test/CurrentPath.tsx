import { useLocation } from 'react-router-dom';

// Renders the router location so tests can assert where a redirect landed.
export const CurrentPath = () => {
  const location = useLocation();
  return <p>at {location.pathname + location.search}</p>;
};
