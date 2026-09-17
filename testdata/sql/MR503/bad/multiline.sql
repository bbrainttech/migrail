UPDATE orders
SET
  status = 'archived',
  archived_at = now();
