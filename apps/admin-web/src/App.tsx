import { lazy, Suspense } from 'react'
import { Spin } from 'antd'
import { Navigate, RouterProvider, createBrowserRouter } from 'react-router'
import { useAuthStore } from './stores/authStore'

const AdminLayout = lazy(() => import('./app/AdminLayout').then((module) => ({ default: module.AdminLayout })))
const DashboardPage = lazy(() => import('./pages/DashboardPage').then((module) => ({ default: module.DashboardPage })))
const LoginPage = lazy(() => import('./pages/LoginPage').then((module) => ({ default: module.LoginPage })))
const PlaceholderPage = lazy(() => import('./pages/PlaceholderPage').then((module) => ({ default: module.PlaceholderPage })))
const SkuListPage = lazy(() => import('./pages/SkuListPage'))
const SkuEditPage = lazy(() => import('./pages/SkuEditPage'))
const OrderListPage = lazy(() => import('./pages/OrderListPage'))
const OrderDetailPage = lazy(() => import('./pages/OrderDetailPage'))
const CategoryPage = lazy(() => import('./pages/CategoryPage'))
const WorkOrderPage = lazy(() => import('./pages/WorkOrderPage'))
const WorkerPage = lazy(() => import('./pages/WorkerPage'))
const SkillPage = lazy(() => import('./pages/SkillPage'))

function ProtectedLayout() {
  const credential = useAuthStore((state) => state.credential)
  return credential ? <AdminLayout /> : <Navigate to="/login" replace />
}

const router = createBrowserRouter([
  { path: '/login', element: <LoginPage /> },
  {
    path: '/',
    element: <ProtectedLayout />,
    children: [
      { index: true, element: <DashboardPage /> },
      { path: 'catalog/skus', element: <SkuListPage /> },
      { path: 'catalog/categories', element: <CategoryPage /> },
      { path: 'catalog/skus/new', element: <SkuEditPage /> },
      { path: 'catalog/skus/:id/edit', element: <SkuEditPage /> },
      { path: 'orders', element: <OrderListPage /> },
      { path: 'orders/:id', element: <OrderDetailPage /> },
      { path: 'work-orders', element: <WorkOrderPage /> },
      { path: 'workers', element: <WorkerPage /> },
      { path: 'worker-skills', element: <SkillPage /> },
      { path: 'customers', element: <PlaceholderPage title="客户资产" /> },
      { path: 'inventory', element: <PlaceholderPage title="材料库存" /> },
      { path: 'after-sales', element: <PlaceholderPage title="售后质保" /> },
      { path: 'enterprises', element: <PlaceholderPage title="企业合同" /> },
      { path: 'settings', element: <PlaceholderPage title="系统设置" /> },
    ],
  },
  { path: '*', element: <Navigate to="/" replace /> },
])

export default function App() {
  return (
    <Suspense fallback={<Spin fullscreen tip="正在加载运维中心" />}>
      <RouterProvider router={router} />
    </Suspense>
  )
}
