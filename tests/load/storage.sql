-- D13 / RN-22 / RN-27: solo agregados de benefits_db, sin exportar cuerpos ni IDs.
-- Ejecutar después de las cargas para no añadir este escaneo a la medición.
SELECT json_build_object(
    'measured_at', now(),
    'scope', 'Toda la base local de Benefits; incluye fixtures de etapas anteriores',
    'database_bytes', pg_database_size(current_database()),
    'movements', (
        SELECT json_build_object(
            'rows', count(*),
            'mean_row_bytes', round(avg(pg_column_size(m)), 2),
            'table_bytes', pg_table_size('movimientos_puntos'),
            'index_bytes', pg_indexes_size('movimientos_puntos'),
            'total_bytes', pg_total_relation_size('movimientos_puntos')
        ) FROM movimientos_puntos m
    ),
    'idempotency', (
        SELECT json_build_object(
            'rows', count(*),
            'mean_row_bytes', round(avg(pg_column_size(o)), 2),
            'table_bytes', pg_table_size('operaciones_partner'),
            'index_bytes', pg_indexes_size('operaciones_partner'),
            'total_bytes', pg_total_relation_size('operaciones_partner')
        ) FROM operaciones_partner o
    )
);
