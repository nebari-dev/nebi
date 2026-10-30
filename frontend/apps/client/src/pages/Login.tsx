import { useEffect } from 'react';
import { useNavigate } from 'react-router-dom';

export const Login = () => {
  const navigate = useNavigate();
  useEffect(() => {
    navigate('/projects');
  }, [navigate]);
  return null;
};
