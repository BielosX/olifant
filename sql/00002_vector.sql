-- +goose Up
-- +goose StatementBegin
CREATE SCHEMA vec;

CREATE DOMAIN vec.vec2 as float8[]
    CHECK (array_length(VALUE, 1) = 2);

CREATE FUNCTION vec.vec2_zero() RETURNS vec.vec2 AS $$
BEGIN
    RETURN ARRAY[0.0, 0.0];
END;
$$ LANGUAGE plpgsql;

CREATE FUNCTION vec.vec2_of(x float8, y float8) RETURNS vec.vec2 AS $$
BEGIN
    RETURN ARRAY[x, y];
END;
$$ LANGUAGE plpgsql;

CREATE FUNCTION vec.vec2_up() RETURNS vec.vec2 AS $$
BEGIN
    RETURN ARRAY[0.0, 1.0];
END;
$$ LANGUAGE plpgsql;

CREATE FUNCTION vec.vec2_down() RETURNS vec.vec2 AS $$
BEGIN
    RETURN ARRAY[0.0, -1.0];
END;
$$ LANGUAGE plpgsql;

CREATE FUNCTION vec.vec2_left() RETURNS vec.vec2 AS $$
BEGIN
    RETURN ARRAY[-1.0, 0.0];
END;
$$ LANGUAGE plpgsql;

CREATE FUNCTION vec.vec2_right() RETURNS vec.vec2 AS $$
BEGIN
    RETURN ARRAY[1.0, 0.0];
END;
$$ LANGUAGE plpgsql;

CREATE FUNCTION vec.vec2_len(v vec.vec2) RETURNS float8 AS $$
BEGIN
    RETURN sqrt(power(v[1], 2) + power(v[2], 2));
END;
$$ LANGUAGE plpgsql;

CREATE FUNCTION vec.vec2_normalize(v vec.vec2) RETURNS vec.vec2 AS $$
DECLARE
    vec_len float8;
BEGIN
    vec_len := vec.vec2_len(v);
    RETURN ARRAY[v[1] / vec_len, v[2] / vec_len];
END;
$$ LANGUAGE plpgsql;

CREATE FUNCTION vec.vec2_add(fst vec.vec2, snd vec.vec2) RETURNS vec.vec2 AS $$
BEGIN
    RETURN ARRAY[fst[1] + snd[1], fst[2] + snd[2]];
END;
$$ LANGUAGE plpgsql;

CREATE FUNCTION vec.vec2_sub(fst vec.vec2, snd vec.vec2) RETURNS vec.vec2 AS $$
BEGIN
    RETURN ARRAY[fst[1] - snd[1], fst[2] - snd[2]];
END;
$$ LANGUAGE plpgsql;

CREATE FUNCTION vec.vec2_mul_scalar(vec vec.vec2, scalar float8) RETURNS vec.vec2 AS $$
BEGIN
    RETURN ARRAY[scalar * vec[1], scalar * vec[2]];
END;
$$ LANGUAGE plpgsql;

-- +goose StatementEnd
