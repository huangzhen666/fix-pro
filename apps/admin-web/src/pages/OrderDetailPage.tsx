import { useNavigate, useParams } from 'react-router'
import { OrderDetailDrawer } from '../components/OrderDetailDrawer'

export default function OrderDetailPage() {
  const { id } = useParams(); const navigate = useNavigate()
  return <OrderDetailDrawer open={Boolean(id)} orderId={id} onClose={() => navigate('/orders')} />
}
