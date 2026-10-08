import { Route, Routes } from 'react-router-dom';
import { expect, it } from 'vitest';
import { renderWithProviders, screen } from '@/test/utils';
import { Login } from './Login';

it('redirects the client login route to projects', async () => {
  renderWithProviders(
    <Routes>
      <Route path="/login" element={<Login />} />
      <Route path="/projects" element={<div>Projects</div>} />
    </Routes>,
    { initialEntries: ['/login'] },
  );
  expect(await screen.findByText('Projects')).toBeInTheDocument();
  expect(screen.queryByLabelText('Password')).not.toBeInTheDocument();
});
