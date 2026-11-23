const { secret, ...rest } = input.result.output
return { ok: true, output: rest }
