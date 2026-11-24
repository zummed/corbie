const { secret, ...rest } = input.output
return { ok: true, output: rest }
